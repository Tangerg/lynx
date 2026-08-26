package agentexec

import (
	"context"
	"fmt"
	"strconv"

	agent "github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	domaintool "github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/core/chatclient"
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
	builder, err := executor.newInteractionDeploymentBuilder(
		ctx, session, start, client, maxModelCalls,
	)
	if err != nil {
		return nil, err
	}
	return builder.build()
}

type interactionDeploymentBuilder struct {
	executor          *InteractionExecutor
	session           *interactionSession
	start             runs.RootExecutionStart
	client            *chatclient.Client
	maxModelCalls     uint32
	delegation        effectiveInteractionDelegation
	maxDepth          uint32
	instructions      []corechat.Message
	rootManifest      toolset.Manifest
	delegatedManifest toolset.Manifest
	deployments       *interactionDeploymentSet
}

func (executor *InteractionExecutor) newInteractionDeploymentBuilder(
	ctx context.Context,
	session *interactionSession,
	start runs.RootExecutionStart,
	client *chatclient.Client,
	maxModelCalls uint32,
) (*interactionDeploymentBuilder, error) {
	delegationConfig, err := effectiveDelegation(executor.config.Delegation)
	if err != nil {
		return nil, err
	}
	instructions, err := interactionInstructionContext(start.WorkingContext)
	if err != nil {
		return nil, err
	}
	rootManifest, err := executor.resolveInteractionManifest(ctx, domaintool.GroupRoot)
	if err != nil {
		return nil, err
	}
	builder := &interactionDeploymentBuilder{
		executor: executor, session: session, start: start, client: client,
		maxModelCalls: maxModelCalls, delegation: delegationConfig,
		instructions: instructions, rootManifest: rootManifest,
		deployments: &interactionDeploymentSet{
			byRef:             make(map[agent.DeploymentRef]agent.Deployment),
			delegatesByParent: make(map[agent.DeploymentRef]map[string]agent.DeploymentRef),
			managedChildren:   make(map[agent.DeploymentRef]struct{}),
			treeLimits:        agent.TreeLimits{MaxDepth: 1, MaxChildren: 1, MaxActiveChildren: 1, MaxTreeProcesses: 1},
		},
	}
	if start.ChildRunAdmissionEnabled {
		builder.maxDepth = delegationConfig.treeLimits.MaxDepth
		builder.deployments.treeLimits = delegationConfig.treeLimits
	}
	if builder.maxDepth > 0 {
		builder.delegatedManifest, err = executor.resolveInteractionManifest(ctx, domaintool.GroupDelegated)
		if err != nil {
			return nil, err
		}
	}
	return builder, nil
}

func (builder *interactionDeploymentBuilder) build() (*interactionDeploymentSet, error) {
	var next agent.Deployment
	for depth := int(builder.maxDepth); depth >= 0; depth-- {
		deployment, err := builder.buildAtDepth(depth, next)
		if err != nil {
			return nil, err
		}
		builder.deployments.byRef[deployment.DeploymentRef()] = deployment
		if depth > 0 {
			builder.deployments.managedChildren[deployment.DeploymentRef()] = struct{}{}
		}
		if next.Valid() {
			builder.deployments.delegatesByParent[deployment.DeploymentRef()] = map[string]agent.DeploymentRef{
				domaintool.DelegateTask: next.DeploymentRef(),
			}
		}
		next = deployment
	}
	builder.deployments.root = next
	return builder.deployments, nil
}

func (builder *interactionDeploymentBuilder) buildAtDepth(depth int, next agent.Deployment) (agent.Deployment, error) {
	role, manifest, definitionName, definitionDescription := builder.layerIdentity(depth)
	delegates, delegateBudget, err := builder.delegateLayer(depth, next)
	if err != nil {
		return agent.Deployment{}, err
	}
	definition, err := interaction.NewDefinition(interaction.DefinitionConfig{
		Name: definitionName, Description: definitionDescription,
		Version: interactionDefinitionVersion, MaxModelCalls: builder.maxModelCalls,
		Delegates: delegates,
	})
	if err != nil {
		return agent.Deployment{}, fmt.Errorf("agentexec: build Interaction definition at depth %d: %w", depth, err)
	}
	visible, deferred := wrapInteractionTools(manifest, builder.session, builder.executor.config, builder.start)
	dispatcher, err := interaction.NewDispatcher(definition, interaction.DispatcherConfig{
		Client: builder.client, Tools: visible, DeferredTools: deferred,
		MaxConcurrentToolCalls: builder.executor.config.MaxConcurrentToolCalls,
		StreamModelResponses:   builder.executor.config.StreamModelResponses,
	})
	if err != nil {
		return agent.Deployment{}, fmt.Errorf("agentexec: build Interaction dispatcher at depth %d: %w", depth, err)
	}
	deploymentDefinition, err := builder.deploymentDefinition(depth, definition)
	if err != nil {
		return agent.Deployment{}, err
	}
	var delegateRef agent.DeploymentRef
	if next.Valid() {
		delegateRef = next.DeploymentRef()
	}
	configuration, err := builder.executor.interactionConfiguration(
		builder.session, builder.start, builder.maxModelCalls, manifest, role, uint32(depth), delegateRef,
		delegateBudget, builder.instructions,
	)
	if err != nil {
		return agent.Deployment{}, err
	}
	deployment, err := agent.NewDeployment(agent.DeploymentConfig{
		Definition:           deploymentDefinition,
		Dispatcher:           &interactionDispatcher{inner: dispatcher, session: builder.session},
		ImplementationDigest: agent.ComputeDigest([]byte(builder.executor.config.ImplementationIdentity)),
		ConfigurationDigest:  agent.ComputeDigest(configuration),
	})
	if err != nil {
		return agent.Deployment{}, fmt.Errorf("agentexec: build Interaction deployment at depth %d: %w", depth, err)
	}
	return deployment, nil
}

func (builder *interactionDeploymentBuilder) layerIdentity(depth int) (string, toolset.Manifest, string, string) {
	if depth == 0 {
		return domaintool.GroupRoot, builder.rootManifest, interactionDefinitionName, interactionDefinitionDescription
	}
	return domaintool.GroupDelegated,
		builder.delegatedManifest,
		"lyra.runtime.interaction.delegate.depth" + strconv.Itoa(depth),
		"Run one isolated delegated Lyra interaction."
}

func (builder *interactionDeploymentBuilder) delegateLayer(
	depth int,
	next agent.Deployment,
) ([]interaction.Delegate, agent.Budget, error) {
	if !next.Valid() {
		return nil, agent.Budget{}, nil
	}
	budget, err := delegateSubtreeBudget(builder.delegation.processBudget, builder.maxDepth-uint32(depth))
	if err != nil {
		return nil, agent.Budget{}, fmt.Errorf("agentexec: allocate Delegate at depth %d: %w", depth, err)
	}
	delegate, err := interaction.NewDelegate(interaction.DelegateConfig{
		Name: domaintool.DelegateTask, Description: delegateDescription,
		Deployment: next, Budget: budget,
	})
	if err != nil {
		return nil, agent.Budget{}, fmt.Errorf("agentexec: build Delegate at depth %d: %w", depth, err)
	}
	return []interaction.Delegate{delegate}, budget, nil
}

func (builder *interactionDeploymentBuilder) deploymentDefinition(
	depth int,
	definition *interaction.Definition,
) (agent.Definition, error) {
	if depth == 0 {
		return definition, nil
	}
	return newDelegatedInteractionDefinition(
		"lyra.runtime.delegated_task.depth"+strconv.Itoa(depth), definition, builder.instructions,
	)
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
		return toolset.Manifest{}, fmt.Errorf("agentexec: resolve Interaction %s Tools: %w", group, err)
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
