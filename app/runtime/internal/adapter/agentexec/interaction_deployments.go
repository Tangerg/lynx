package agentexec

import (
	"context"
	"fmt"
	"strconv"

	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/interaction"
	"github.com/Tangerg/scope/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/scope/app/runtime/internal/application/runs"
	domaintool "github.com/Tangerg/scope/app/runtime/internal/domain/tool"
	corechat "github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

type interactionDeploymentSet struct {
	root              agent.Deployment
	byRef             map[agent.DeploymentRef]agent.Deployment
	delegatesByParent map[agent.DeploymentRef]map[string]agent.DeploymentRef
	managedChildren   map[agent.DeploymentRef]struct{}
	treeLimits        agent.TreeLimits
}

func (i *interactionDeploymentSet) Resolve(
	reference agent.DeploymentRef,
) (agent.Deployment, error) {
	if i == nil {
		return agent.Deployment{}, agent.ErrInvalidDeploymentRef
	}
	deployment, found := i.byRef[reference]
	if !found {
		return agent.Deployment{}, agent.ErrInvalidDeploymentRef
	}
	return deployment, nil
}

func (i *interactionDeploymentSet) managedChild(reference agent.DeploymentRef) bool {
	_, found := i.managedChildren[reference]
	return found
}

func (i *interactionDeploymentSet) delegateTarget(
	parent agent.DeploymentRef,
	name string,
) (agent.DeploymentRef, bool) {
	target, found := i.delegatesByParent[parent][name]
	return target, found
}

func (i *InteractionExecutor) buildInteractionDeployments(
	ctx context.Context,
	session *interactionSession,
	start runs.RootExecutionStart,
	client *chatclient.Client,
	maxModelCalls uint32,
) (*interactionDeploymentSet, error) {
	builder, err := i.newInteractionDeploymentBuilder(
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

func (i *InteractionExecutor) newInteractionDeploymentBuilder(
	ctx context.Context,
	session *interactionSession,
	start runs.RootExecutionStart,
	client *chatclient.Client,
	maxModelCalls uint32,
) (*interactionDeploymentBuilder, error) {
	delegationConfig, err := effectiveDelegation(i.config.Delegation)
	if err != nil {
		return nil, err
	}
	instructions, err := interactionInstructionContext(start.WorkingContext)
	if err != nil {
		return nil, err
	}
	rootManifest, err := i.resolveInteractionManifest(ctx, domaintool.GroupRoot)
	if err != nil {
		return nil, err
	}
	builder := &interactionDeploymentBuilder{
		executor: i, session: session, start: start, client: client,
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
		builder.delegatedManifest, err = i.resolveInteractionManifest(ctx, domaintool.GroupDelegated)
		if err != nil {
			return nil, err
		}
	}
	return builder, nil
}

func (i *interactionDeploymentBuilder) build() (*interactionDeploymentSet, error) {
	var next agent.Deployment
	for depth := int(i.maxDepth); depth >= 0; depth-- {
		deployment, err := i.buildAtDepth(depth, next)
		if err != nil {
			return nil, err
		}
		i.deployments.byRef[deployment.DeploymentRef()] = deployment
		if depth > 0 {
			i.deployments.managedChildren[deployment.DeploymentRef()] = struct{}{}
		}
		if next.Valid() {
			i.deployments.delegatesByParent[deployment.DeploymentRef()] = map[string]agent.DeploymentRef{
				domaintool.DelegateTask: next.DeploymentRef(),
			}
		}
		next = deployment
	}
	i.deployments.root = next
	return i.deployments, nil
}

func (i *interactionDeploymentBuilder) buildAtDepth(depth int, next agent.Deployment) (agent.Deployment, error) {
	group, manifest, definitionName, definitionDescription := i.layerIdentity(depth)
	delegates, delegateBudget, err := i.delegateLayer(depth, next)
	if err != nil {
		return agent.Deployment{}, err
	}
	definition, err := interaction.NewDefinition(interaction.DefinitionConfig{
		Name: definitionName, Description: definitionDescription,
		Version: interactionDefinitionVersion, MaxModelCalls: i.maxModelCalls,
		Delegates: delegates,
	})
	if err != nil {
		return agent.Deployment{}, fmt.Errorf("agentexec: build Interaction definition at depth %d: %w", depth, err)
	}
	visible, deferred := wrapInteractionTools(manifest, i.session, i.executor.config, i.start)
	var contextReducer interaction.ModelContextReducer
	if i.executor.config.ModelContextCompactor != nil {
		var counter ModelContextInputTokenCounter
		if i.client.SupportsInputTokenCounting() {
			counter = i.client
		}
		contextReducer = newInteractionModelContextReducer(
			i.executor.config.ModelContextCompactor,
			i.executor.config.ModelContextState,
			i.session,
			i.start,
			i.instructions,
			counter,
		)
	}
	dispatcher, err := interaction.NewDispatcher(definition, interaction.DispatcherConfig{
		Client: i.client, Tools: visible, DeferredTools: deferred,
		MaxConcurrentToolCalls: i.executor.config.MaxConcurrentToolCalls,
		StreamModelResponses:   i.executor.config.StreamModelResponses,
		ModelContextReducer:    contextReducer,
	})
	if err != nil {
		return agent.Deployment{}, fmt.Errorf("agentexec: build Interaction dispatcher at depth %d: %w", depth, err)
	}
	deploymentDefinition, err := i.deploymentDefinition(depth, definition)
	if err != nil {
		return agent.Deployment{}, err
	}
	var delegateRef agent.DeploymentRef
	if next.Valid() {
		delegateRef = next.DeploymentRef()
	}
	configuration, err := i.executor.interactionConfiguration(
		i.session, i.start, i.maxModelCalls, manifest, group, uint32(depth), delegateRef,
		delegateBudget, i.instructions,
	)
	if err != nil {
		return agent.Deployment{}, err
	}
	deployment, err := agent.NewDeployment(agent.DeploymentConfig{
		Definition:           deploymentDefinition,
		Dispatcher:           &interactionDispatcher{inner: dispatcher, session: i.session},
		ImplementationDigest: agent.ComputeDigest([]byte(i.executor.config.ImplementationIdentity)),
		ConfigurationDigest:  agent.ComputeDigest(configuration),
	})
	if err != nil {
		return agent.Deployment{}, fmt.Errorf("agentexec: build Interaction deployment at depth %d: %w", depth, err)
	}
	return deployment, nil
}

func (i *interactionDeploymentBuilder) layerIdentity(
	depth int,
) (domaintool.Group, toolset.Manifest, string, string) {
	if depth == 0 {
		return domaintool.GroupRoot, i.rootManifest, interactionDefinitionName, interactionDefinitionDescription
	}
	return domaintool.GroupDelegated,
		i.delegatedManifest,
		"scopeapp.runtime.interaction.delegate.depth" + strconv.Itoa(depth),
		"Run one isolated delegated ScopeApp interaction."
}

func (i *interactionDeploymentBuilder) delegateLayer(
	depth int,
	next agent.Deployment,
) ([]interaction.Delegate, agent.Budget, error) {
	if !next.Valid() {
		return nil, agent.Budget{}, nil
	}
	budget, err := delegateSubtreeBudget(i.delegation.processBudget, i.maxDepth-uint32(depth))
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

func (i *interactionDeploymentBuilder) deploymentDefinition(
	depth int,
	definition *interaction.Definition,
) (agent.Definition, error) {
	if depth == 0 {
		return definition, nil
	}
	return newDelegatedInteractionDefinition(
		"scopeapp.runtime.delegated_task.depth"+strconv.Itoa(depth), definition, i.instructions,
	)
}

func (i *InteractionExecutor) resolveInteractionManifest(
	ctx context.Context,
	group domaintool.Group,
) (toolset.Manifest, error) {
	if i.config.ToolResolver == nil {
		return toolset.Manifest{}, nil
	}
	manifest, err := i.config.ToolResolver.Manifest(ctx, group)
	if err != nil {
		return toolset.Manifest{}, fmt.Errorf("agentexec: resolve Interaction %s Tools: %w", group, err)
	}
	manifest = manifest.Clone()
	if err := validateToolManifest(manifest); err != nil {
		return toolset.Manifest{}, err
	}
	if err := i.validateInteractionTools(manifest); err != nil {
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
		provenance, found, decodeErr := messages[index].Metadata.Decode[contextProvenance](
			contextProvenanceMetadataKey,
		)
		if decodeErr != nil {
			return nil, fmt.Errorf("agentexec: decode Interaction instruction[%d] provenance: %w", index, decodeErr)
		}
		if !found {
			break
		}
		if validationErr := provenance.validate(); validationErr != nil {
			return nil, fmt.Errorf("agentexec: Interaction instruction[%d] provenance: %w", index, validationErr)
		}
		_, sessionState, err := provenance.replaceableSessionState()
		if err != nil {
			return nil, fmt.Errorf("agentexec: Interaction instruction[%d] provenance: %w", index, err)
		}
		if sessionState {
			break
		}
		instructions = append(instructions, messages[index].Clone())
	}
	return instructions, nil
}
