package agentexec

import (
	"context"
	"fmt"
	"strconv"

	agent "github.com/Tangerg/lynx/agent2"
	"github.com/Tangerg/lynx/agent2/interaction"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/catalog"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/delegation"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	domaintool "github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/chatclient"
	corechat "github.com/Tangerg/lynx/core/chat"
)

type interactionDeploymentSet struct {
	root              agent.Deployment
	byRef             map[agent.DeploymentRef]agent.Deployment
	delegatesByParent map[agent.DeploymentRef]map[string]agent.DeploymentRef
	managedChildren   map[agent.DeploymentRef]struct{}
	treeLimits        agent.TreeLimits
}

func (set *interactionDeploymentSet) Resolve(
	reference agent.DeploymentRef,
) (agent.Deployment, error) {
	if set == nil {
		return agent.Deployment{}, agent.ErrInvalidDeploymentRef
	}
	deployment, found := set.byRef[reference]
	if !found {
		return agent.Deployment{}, agent.ErrInvalidDeploymentRef
	}
	return deployment, nil
}

func (set *interactionDeploymentSet) managedChild(reference agent.DeploymentRef) bool {
	_, found := set.managedChildren[reference]
	return found
}

func (set *interactionDeploymentSet) delegateTarget(
	parent agent.DeploymentRef,
	name string,
) (agent.DeploymentRef, bool) {
	target, found := set.delegatesByParent[parent][name]
	return target, found
}

func (executor *InteractionExecutor) buildInteractionDeployments(
	ctx context.Context,
	session *interactionSession,
	start runs.RootExecutionStart,
	client *chatclient.Client,
	maxModelCalls uint32,
) (*interactionDeploymentSet, error) {
	delegationConfig, err := effectiveDelegation(executor.config.Delegation)
	if err != nil {
		return nil, err
	}
	set := &interactionDeploymentSet{
		byRef:             make(map[agent.DeploymentRef]agent.Deployment),
		delegatesByParent: make(map[agent.DeploymentRef]map[string]agent.DeploymentRef),
		managedChildren:   make(map[agent.DeploymentRef]struct{}),
		treeLimits:        agent.TreeLimits{MaxDepth: 1, MaxChildren: 1, MaxActiveChildren: 1, MaxTreeProcesses: 1},
	}
	maxDepth := uint32(0)
	if start.ChildRunAdmissionEnabled {
		maxDepth = delegationConfig.treeLimits.MaxDepth
		set.treeLimits = delegationConfig.treeLimits
	}
	instructions, err := interactionInstructionContext(start.WorkingContext)
	if err != nil {
		return nil, err
	}
	rootManifest, err := executor.resolveInteractionManifest(ctx, domaintool.GroupRoot)
	if err != nil {
		return nil, err
	}
	delegatedManifest := toolset.Manifest{}
	if maxDepth > 0 {
		delegatedManifest, err = executor.resolveInteractionManifest(ctx, domaintool.GroupDelegated)
		if err != nil {
			return nil, err
		}
	}

	var next agent.Deployment
	for depth := int(maxDepth); depth >= 0; depth-- {
		role := domaintool.GroupDelegated
		manifest := delegatedManifest
		definitionName := "lyra.runtime.interaction.delegate.depth" + strconv.Itoa(depth)
		definitionDescription := "Run one isolated delegated Lyra interaction."
		if depth == 0 {
			role = domaintool.GroupRoot
			manifest = rootManifest
			definitionName = interactionDefinitionName
			definitionDescription = interactionDefinitionDescription
		}
		var delegates []interaction.Delegate
		var delegateBudget agent.Budget
		if next.Valid() {
			delegateBudget, err = delegateSubtreeBudget(
				delegationConfig.processBudget,
				maxDepth-uint32(depth),
			)
			if err != nil {
				return nil, fmt.Errorf("agentexec: allocate native Delegate at depth %d: %w", depth, err)
			}
			delegate, err := interaction.NewDelegate(interaction.DelegateConfig{
				Name: catalog.DelegateTask, Description: delegation.Description,
				Deployment: next, Budget: delegateBudget,
			})
			if err != nil {
				return nil, fmt.Errorf("agentexec: build native Delegate at depth %d: %w", depth, err)
			}
			delegates = []interaction.Delegate{delegate}
		}
		definition, err := interaction.NewDefinition(interaction.DefinitionConfig{
			Name: definitionName, Description: definitionDescription,
			Version: interactionDefinitionVersion, MaxModelCalls: maxModelCalls,
			Delegates: delegates,
		})
		if err != nil {
			return nil, fmt.Errorf("agentexec: build native Interaction definition at depth %d: %w", depth, err)
		}
		visible, deferred := wrapInteractionTools(manifest, session, executor.config, start)
		dispatcher, err := interaction.NewDispatcher(definition, interaction.DispatcherConfig{
			Client: client, Tools: visible, DeferredTools: deferred,
			MaxConcurrentToolCalls: executor.config.MaxConcurrentToolCalls,
			StreamModelResponses:   executor.config.StreamModelResponses,
		})
		if err != nil {
			return nil, fmt.Errorf("agentexec: build native Interaction dispatcher at depth %d: %w", depth, err)
		}
		var deploymentDefinition agent.Definition = definition
		if depth > 0 {
			deploymentDefinition, err = newDelegatedInteractionDefinition(
				"lyra.runtime.delegated_task.depth"+strconv.Itoa(depth), definition, instructions,
			)
			if err != nil {
				return nil, err
			}
		}
		var delegateRef agent.DeploymentRef
		if next.Valid() {
			delegateRef = next.DeploymentRef()
		}
		configuration, err := executor.interactionConfiguration(
			session, start, maxModelCalls, manifest, role, uint32(depth), delegateRef, delegateBudget,
			instructions,
		)
		if err != nil {
			return nil, err
		}
		deployment, err := agent.NewDeployment(agent.DeploymentConfig{
			Definition:           deploymentDefinition,
			Dispatcher:           &interactionDispatcher{inner: dispatcher, session: session},
			ImplementationDigest: agent.ComputeDigest([]byte(executor.config.ImplementationIdentity)),
			ConfigurationDigest:  agent.ComputeDigest(configuration),
		})
		if err != nil {
			return nil, fmt.Errorf("agentexec: build native Interaction deployment at depth %d: %w", depth, err)
		}
		set.byRef[deployment.DeploymentRef()] = deployment
		if depth > 0 {
			set.managedChildren[deployment.DeploymentRef()] = struct{}{}
		}
		if next.Valid() {
			set.delegatesByParent[deployment.DeploymentRef()] = map[string]agent.DeploymentRef{
				catalog.DelegateTask: next.DeploymentRef(),
			}
		}
		next = deployment
	}
	set.root = next
	return set, nil
}

func (executor *InteractionExecutor) resolveInteractionManifest(
	ctx context.Context,
	group string,
) (toolset.Manifest, error) {
	if executor.config.ToolResolver == nil {
		return toolset.Manifest{}, nil
	}
	manifest, err := executor.config.ToolResolver.Manifest(ctx, group)
	if err != nil {
		return toolset.Manifest{}, fmt.Errorf("agentexec: resolve native Interaction %s Tools: %w", group, err)
	}
	manifest = manifest.Clone()
	if err := validateToolManifest(manifest); err != nil {
		return toolset.Manifest{}, err
	}
	if err := executor.validateInteractionTools(manifest); err != nil {
		return toolset.Manifest{}, err
	}
	return manifest, nil
}

func interactionInstructionContext(messages []corechat.Message) ([]corechat.Message, error) {
	var instructions []corechat.Message
	for index := range messages {
		if messages[index].Role != corechat.RoleSystem {
			break
		}
		if err := messages[index].Validate(); err != nil {
			return nil, fmt.Errorf("agentexec: invalid Interaction instruction[%d]: %w", index, err)
		}
		instructions = append(instructions, messages[index].Clone())
	}
	return instructions, nil
}
