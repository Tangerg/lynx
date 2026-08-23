package agentexec

import (
	"context"
	"errors"
	"fmt"

	agent "github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/chatclient"
	toolcontract "github.com/Tangerg/lynx/tool"
)

const (
	delegationMaxDepth          = 3
	delegationMaxChildren       = 12
	delegationMaxActiveChildren = 4
	delegationMaxTreeProcesses  = 40
	delegationBaseSteps         = 128
	delegationBaseEffects       = 128
	delegationBaseSignals       = 1024
)

type deploymentFamily struct {
	root     agent.Deployment
	values   map[agent.DeploymentRef]agent.Deployment
	targets  map[agent.DeploymentRef]agent.DeploymentRef
	limits   agent.TreeLimits
}

func (family *deploymentFamily) Resolve(reference agent.DeploymentRef) (agent.Deployment, error) {
	if family == nil {
		return agent.Deployment{}, agent.ErrInvalidDeployment
	}
	deployment, found := family.values[reference]
	if !found {
		return agent.Deployment{}, fmt.Errorf("%w: unknown delegated deployment", agent.ErrInvalidDeployment)
	}
	return deployment, nil
}

type familyConfig struct {
	client             *chatclient.Client
	provider, model    string
	workspace          string
	maxModelCalls      uint32
	rootTools          []ExecutableTool
	childManifest      []ExecutableTool
	toolRouter         *runToolRouter
	observer           *executionObserver
}

func newDeploymentFamily(config familyConfig) (*deploymentFamily, error) {
	if config.client == nil || config.observer == nil || config.toolRouter == nil || config.maxModelCalls == 0 {
		return nil, errors.New("agentexec: delegated deployment family is incomplete")
	}
	capabilities, err := agent.NewCapabilitySet()
	if err != nil {
		return nil, err
	}
	values := make(map[agent.DeploymentRef]agent.Deployment, delegationMaxDepth+1)
	targets := make(map[agent.DeploymentRef]agent.DeploymentRef, delegationMaxDepth)
	children := make([]agent.Deployment, delegationMaxDepth+1)
	routedTools := routedManifest(config.childManifest, config.toolRouter)
	childVisible, childDeferred := partitionToolManifest(routedTools)
	childToolDigest, err := toolManifestDigest(config.childManifest)
	if err != nil {
		return nil, err
	}
	var next agent.Deployment
	for depth := delegationMaxDepth; depth >= 1; depth-- {
		delegates := []interaction.Delegate{}
		if next.Valid() {
			budget, budgetErr := delegationBudget(uint64(delegationMaxDepth - depth))
			if budgetErr != nil {
				return nil, budgetErr
			}
			delegate, delegateErr := interaction.NewDelegate(interaction.DelegateConfig{
				Name: DelegateToolName,
				Description: "Delegate an independent, well-scoped task when parallel or isolated work is useful.",
				Deployment: next, Budget: budget, Capabilities: capabilities,
			})
			if delegateErr != nil {
				return nil, fmt.Errorf("agentexec: bind nested Delegate at depth %d: %w", depth, delegateErr)
			}
			delegates = append(delegates, delegate)
		}
		inner, defineErr := interaction.NewDefinition(interaction.DefinitionConfig{
			Name: fmt.Sprintf("lyra.interaction.delegated.depth_%d", depth),
			Description: "Complete one delegated task using only the authority assigned to this worker.",
			Version: "2.0.0", MaxModelCalls: config.maxModelCalls, Delegates: delegates,
		})
		if defineErr != nil {
			return nil, fmt.Errorf("agentexec: define delegated Interaction at depth %d: %w", depth, defineErr)
		}
		dispatcher, dispatchErr := interaction.NewDispatcher(inner, interaction.DispatcherConfig{
			Client: config.client, Tools: cloneTools(childVisible),
			DeferredTools: cloneTools(childDeferred),
			StreamModelResponses: true, Observer: config.observer,
		})
		if dispatchErr != nil {
			return nil, fmt.Errorf("agentexec: dispatch delegated Interaction at depth %d: %w", depth, dispatchErr)
		}
		definition, defineErr := newDelegatedDefinition(
			fmt.Sprintf("lyra.delegated_task.depth_%d", depth), inner,
		)
		if defineErr != nil {
			return nil, defineErr
		}
		deployment, deploymentErr := agent.NewDeployment(agent.DeploymentConfig{
			Definition: definition, Dispatcher: dispatcher,
			ImplementationDigest: agent.ComputeDigest([]byte("lyra-app2-delegated-interaction-v2")),
			ConfigurationDigest: agent.ComputeDigest([]byte(fmt.Sprintf(
				"%s\x00%s\x00%s\x00depth:%d\x00tools:%s",
				config.provider, config.model, config.workspace, depth, childToolDigest.String(),
			))),
		})
		if deploymentErr != nil {
			return nil, fmt.Errorf("agentexec: deploy delegated Interaction at depth %d: %w", depth, deploymentErr)
		}
		children[depth] = deployment
		values[deployment.DeploymentRef()] = deployment
		if next.Valid() {
			targets[deployment.DeploymentRef()] = next.DeploymentRef()
		}
		next = deployment
	}
	rootBudget, err := delegationBudget(delegationMaxDepth - 1)
	if err != nil {
		return nil, err
	}
	rootDelegate, err := interaction.NewDelegate(interaction.DelegateConfig{
		Name: DelegateToolName,
		Description: "Delegate an independent, well-scoped task when parallel or isolated work is useful.",
		Deployment: children[1], Budget: rootBudget, Capabilities: capabilities,
	})
	if err != nil {
		return nil, fmt.Errorf("agentexec: bind root Delegate: %w", err)
	}
	rootDefinition, err := interaction.NewDefinition(interaction.DefinitionConfig{
		Name: "lyra.interaction", Description: "Complete the user's request using the available tools.",
		Version: "4.0.0", MaxModelCalls: config.maxModelCalls,
		Delegates: []interaction.Delegate{rootDelegate},
	})
	if err != nil {
		return nil, fmt.Errorf("agentexec: define delegated root: %w", err)
	}
	rootVisible, rootDeferred := partitionToolManifest(config.rootTools)
	rootToolDigest, err := toolManifestDigest(config.rootTools)
	if err != nil {
		return nil, err
	}
	rootDispatcher, err := interaction.NewDispatcher(rootDefinition, interaction.DispatcherConfig{
		Client: config.client, Tools: rootVisible, DeferredTools: rootDeferred,
		StreamModelResponses: true, Observer: config.observer,
	})
	if err != nil {
		return nil, fmt.Errorf("agentexec: dispatch delegated root: %w", err)
	}
	root, err := agent.NewDeployment(agent.DeploymentConfig{
		Definition: rootDefinition, Dispatcher: rootDispatcher,
		ImplementationDigest: agent.ComputeDigest([]byte("lyra-app2-interaction-v4")),
		ConfigurationDigest: agent.ComputeDigest([]byte(
			config.provider + "\x00" + config.model + "\x00" + config.workspace +
				"\x00delegation\x00tools:" + rootToolDigest.String(),
		)),
	})
	if err != nil {
		return nil, fmt.Errorf("agentexec: deploy delegated root: %w", err)
	}
	values[root.DeploymentRef()] = root
	targets[root.DeploymentRef()] = children[1].DeploymentRef()
	return &deploymentFamily{
		root: root, values: values, targets: targets,
		limits: agent.TreeLimits{
			MaxDepth: delegationMaxDepth, MaxChildren: delegationMaxChildren,
			MaxActiveChildren: delegationMaxActiveChildren, MaxTreeProcesses: delegationMaxTreeProcesses,
		},
	}, nil
}

func delegationBudget(descendantLevels uint64) (agent.Budget, error) {
	levels := descendantLevels + 1
	return agent.NewBudget(
		delegationBaseSteps*levels,
		delegationBaseEffects*levels,
		delegationBaseSignals*levels,
	)
}

func cloneTools(values []toolcontract.Tool) []toolcontract.Tool {
	return append(make([]toolcontract.Tool, 0, len(values)), values...)
}

var _ agent.DeploymentResolver = (*deploymentFamily)(nil)

// A child Tool manifest is assembled before any child has a product identity;
// its values are used only as frozen definitions and capability templates.
func (executor *Executor) childToolManifest(
	ctx context.Context,
	sessionID, workspace string,
	observer *executionObserver,
) ([]ExecutableTool, error) {
	if executor.tools == nil {
		return nil, nil
	}
	return executor.tools.ForRun(ctx, ToolScope{
		SessionID: sessionID, RunID: "delegated_manifest", Workspace: workspace,
		IsRootRun: false, Facts: observer,
	})
}
