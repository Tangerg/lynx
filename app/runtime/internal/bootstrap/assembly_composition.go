package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Tangerg/scope/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/scope/app/runtime/internal/adapter/persistence"
	"github.com/Tangerg/scope/app/runtime/internal/adapter/promptsource"
	"github.com/Tangerg/scope/app/runtime/internal/adapter/skillproposal"
	"github.com/Tangerg/scope/app/runtime/internal/adapter/toolset"
	checkpointstore "github.com/Tangerg/scope/app/runtime/internal/adapter/workspace"
	"github.com/Tangerg/scope/app/runtime/internal/adapter/workspacepath"
	agentmemoryapp "github.com/Tangerg/scope/app/runtime/internal/application/agentmemory"
	"github.com/Tangerg/scope/app/runtime/internal/application/approvals"
	"github.com/Tangerg/scope/app/runtime/internal/application/goals"
	"github.com/Tangerg/scope/app/runtime/internal/application/invalidation"
	planapp "github.com/Tangerg/scope/app/runtime/internal/application/plans"
	"github.com/Tangerg/scope/app/runtime/internal/application/schedules"
	"github.com/Tangerg/scope/app/runtime/internal/application/workspace"
	"github.com/Tangerg/scope/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/scope/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/scope/app/runtime/internal/domain/skills"
	"github.com/Tangerg/scope/app/runtime/internal/domain/tool"
	"github.com/Tangerg/scope/app/runtime/internal/infra/skillauthoring"
)

// policyComposition contains the application policies that share the same
// process-local invalidation vocabulary. It owns no background task or closer.
type policyComposition struct {
	invalidations notificationRelay[invalidation.Notice]
	approvals     *approvals.RuntimePolicy
	goals         goals.Store
	goalReader    *goals.Reader
	goalReporter  *goals.OutcomeReporter
	plans         *planapp.Coordinator
	mcp           mcpEnvironment
	schedules     *schedules.Coordinator
}

func buildPolicyComposition(ctx context.Context, cfg Config) (policyComposition, error) {
	invalidations := newNotificationRelay[invalidation.Notice]()
	approvalPolicy, err := approvals.NewRuntimePolicy(
		cfg.ApprovalMode,
		cfg.ApprovalRuleStore,
		cfg.PermissionModeStore,
		invalidations.Publish,
	)
	if err != nil {
		return policyComposition{}, fmt.Errorf("runtime: approval policy: %w", err)
	}
	mcpSettings, err := buildMCPEnvironment(ctx, cfg.MCPRegistry)
	if err != nil {
		return policyComposition{}, err
	}
	goalStore := goals.WithInvalidations(cfg.GoalStore, invalidations.Publish)
	return policyComposition{
		invalidations: invalidations,
		approvals:     approvalPolicy,
		goals:         goalStore,
		goalReader:    goals.NewReader(goalStore),
		goalReporter:  goals.NewOutcomeReporter(goalStore),
		plans: planapp.New(planapp.Dependencies{
			Store: cfg.PlanStore, Now: time.Now, Invalidations: invalidations.Publish,
		}),
		mcp: mcpSettings,
		schedules: schedules.New(schedules.Dependencies{
			Store:         cfg.ScheduleStore,
			Paths:         workspacepath.Resolver{},
			Invalidations: invalidations.Publish,
		}),
	}, nil
}

// workspaceComposition is the complete authored-workspace capability. All
// members share one resolved scope and one authored-resource observer.
type workspaceComposition struct {
	scope            *workspace.Scope
	agentMemory      *agentmemoryapp.Coordinator
	memoryCuration   *agentmemoryapp.Curation
	authoredWatch    *workspace.AuthoredWatch
	knowledge        *workspace.Knowledge
	skills           *workspace.Skills
	skillMaintenance *workspace.SkillMaintenance
	skillStore       *skillauthoring.Store
	checkpoints      *checkpointstore.Checkpoints
}

func buildWorkspaceComposition(
	cfg Config,
	publish invalidation.Publish,
) (workspaceComposition, error) {
	scope := workspace.NewScope(cfg.DefaultWorkspacePath, cfg.UserHome, workspacepath.Resolver{})
	authoredWatcher, err := checkpointstore.NewAuthoredWatcher(
		cfg.KnowledgeDirectory,
		cfg.UserHome,
		cfg.SkillsUserDir,
	)
	if err != nil {
		return workspaceComposition{}, fmt.Errorf("runtime: build authored resource watcher: %w", err)
	}
	authoredWatch := workspace.NewAuthoredWatch(scope, workspacepath.Resolver{}, authoredWatcher)
	knowledge := workspace.NewKnowledge(
		scope,
		workspacepath.Resolver{},
		cfg.KnowledgeStore,
		authoredWatch,
		publish,
	)
	skillStore := skillauthoring.NewStore(cfg.SkillsUserDir, skills.ScopeUser)
	var skillCurator workspace.SkillCurator
	var idleSkillSweeper workspace.IdleSkillSweeper
	if skillStore.Enabled() {
		skillCurator = skillStore
		idleSkillSweeper = skillStore
	}
	workspaceSkills := workspace.NewSkills(
		scope,
		promptsource.NewWorkspaceSkills(cfg.SkillsUserDir),
		skillCurator,
		skillproposal.NewLibraries(skillStore),
		authoredWatch,
		publish,
	)
	return workspaceComposition{
		scope: scope,
		agentMemory: agentmemoryapp.New(agentmemoryapp.Config{
			Store: cfg.AgentMemoryStore, Roots: scope, Invalidations: publish,
		}),
		memoryCuration: agentmemoryapp.NewCuration(agentmemoryapp.CurationConfig{
			Store: cfg.AgentMemoryStore, Invalidations: publish,
		}),
		authoredWatch: authoredWatch,
		knowledge:     knowledge,
		skills:        workspaceSkills,
		skillMaintenance: workspace.NewSkillMaintenance(
			idleSkillSweeper,
			authoredWatch,
			publish,
		),
		skillStore:  skillStore,
		checkpoints: checkpointstore.NewCheckpoints(cfg.CheckpointDir),
	}, nil
}

// executionComposition owns the model/tool execution graph. Every acquired
// closer is transferred to hostLifetime before an error can escape.
type executionComposition struct {
	conversation    conversationEnvironment
	models          modelEnvironment
	tools           toolEnvironment
	workingContexts *agentexec.WorkingContextComposer
	executor        *agentexec.InteractionExecutor
	toolRegistry    toolset.DiagnosticRegistry
}

func buildExecutionComposition(
	ctx context.Context,
	cfg Config,
	lifetime *hostLifetime,
	buildTools toolEnvironmentBuilder,
	policy policyComposition,
	workspaceServices workspaceComposition,
) (executionComposition, error) {
	conversation, err := buildConversationEnvironment(
		cfg.ConversationStore,
		persistence.NewConversationCompactions(
			cfg.ConversationStore,
			cfg.RunStore,
			persistence.Transactor(cfg.Transactor),
		),
	)
	if err != nil {
		return executionComposition{}, err
	}
	modelServices, err := buildModelEnvironment(ctx, cfg)
	if err != nil {
		return executionComposition{}, err
	}
	toolRuntime, err := buildTools(ctx, toolEnvironmentDependencies{
		lifetime:            lifetime.context,
		config:              cfg,
		approvalPolicy:      policy.approvals,
		mcp:                 policy.mcp,
		agentMemorySearcher: modelServices.agentMemorySearch,
		schedules:           policy.schedules,
		goalReader:          policy.goalReader,
		goalReporter:        policy.goalReporter,
		plan:                policy.plans,
		skillStore:          workspaceServices.skillStore,
		skillProposals:      workspaceServices.skills,
	})
	lifetime.toolResources = slices.Clone(toolRuntime.closers)
	if err != nil {
		return executionComposition{}, err
	}
	workingContexts := agentexec.NewWorkingContextComposer(agentexec.WorkingContextConfig{
		UserHome:          cfg.UserHome,
		Knowledge:         workspaceServices.knowledge,
		AgentMemory:       cfg.AgentMemoryStore,
		AgentMemorySearch: modelServices.agentMemorySearch,
		Plan:              cfg.PlanStore,
		Hooks:             cfg.HooksResolver,
	})
	toolAuthorizer, err := agentexec.NewToolAuthorizer(policy.approvals)
	if err != nil {
		return executionComposition{}, fmt.Errorf("runtime: Tool authorizer: %w", err)
	}
	runMaintenance, modelContextCompactor := buildRunMaintenance(
		cfg,
		conversation,
		toolRuntime.tools.Shells,
		workspaceServices.skills,
		workspaceServices.skillMaintenance,
		workspaceServices.memoryCuration,
		modelServices.utilityClient,
	)
	defaultSelection, err := runtimeDefaultModelSelection(cfg)
	if err != nil {
		return executionComposition{}, err
	}
	interactionConfig := agentexec.InteractionExecutorConfig{
		Lifetime:               lifetime.context,
		BuildID:                cfg.BuildID,
		DefaultClient:          cfg.ChatClient,
		DefaultSelection:       defaultSelection,
		ChatResolver:           modelServices.chatResolver,
		ImplementationIdentity: cfg.BuildID,
		ConfigurationIdentity:  "scopeapp.runtime.interaction.v1",
		StreamModelResponses:   true,
		MaxConcurrentToolCalls: 8,
		ToolInterpreter:        toolset.NewInterpreter(policy.plans),
		ToolPresenter:          toolset.Presenter{},
		ToolAuthorizer:         toolAuthorizer,
		ToolHooks:              workingContexts,
		MCPToolAutoApproved: func(server, toolName string) bool {
			return policy.mcp.policy.ToolAutoApproved(mcpserver.ToolRef{Server: server, Tool: toolName})
		},
		Maintenance:           runMaintenance,
		ModelContextCompactor: modelContextCompactor,
		LifecycleHooks:        workingContexts,
		Pricing:               cfg.Pricing,
		Provider:              cfg.Provider,
	}
	if toolRuntime.tools.Resolver != nil {
		interactionConfig.ToolResolver = toolRuntime.tools.Resolver
	}
	if cfg.ToolResultStore != nil {
		interactionConfig.ToolResultStore = cfg.ToolResultStore
		interactionConfig.ToolResultThreshold = cfg.ToolResultThreshold
		interactionConfig.ToolResultReaderName = tool.ReadToolResult
	}
	interactionExecutor, err := agentexec.NewInteractionExecutor(interactionConfig)
	if err != nil {
		return executionComposition{}, fmt.Errorf("runtime: Interaction executor: %w", err)
	}
	lifetime.executor = interactionExecutor
	return executionComposition{
		conversation:    conversation,
		models:          modelServices,
		tools:           toolRuntime,
		workingContexts: workingContexts,
		executor:        interactionExecutor,
		toolRegistry:    toolset.NewDiagnosticRegistry(),
	}, nil
}

func runtimeDefaultModelSelection(cfg Config) (modelref.Selection, error) {
	selection, err := modelref.New(cfg.Provider, cfg.Model)
	if err != nil {
		return modelref.Selection{}, fmt.Errorf("runtime: default model selection: %w", err)
	}
	if !selection.Configured() {
		return modelref.Selection{}, errors.New("runtime: configured default model selection is required")
	}
	return selection, nil
}
