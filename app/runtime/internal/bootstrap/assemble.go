package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/isolation"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/modelcatalog"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/notification"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/persistence"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/promptsource"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/runrecovery"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/runsegment"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/sessiontitle"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/skillproposal"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolname"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/builtin"
	checkpointstore "github.com/Tangerg/lynx/app/runtime/internal/adapter/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/workspacepath"
	agentmemoryapp "github.com/Tangerg/lynx/app/runtime/internal/application/agentmemory"
	"github.com/Tangerg/lynx/app/runtime/internal/application/approvals"
	"github.com/Tangerg/lynx/app/runtime/internal/application/codebase"
	feedbackapp "github.com/Tangerg/lynx/app/runtime/internal/application/feedback"
	"github.com/Tangerg/lynx/app/runtime/internal/application/goals"
	"github.com/Tangerg/lynx/app/runtime/internal/application/invalidation"
	mcpapp "github.com/Tangerg/lynx/app/runtime/internal/application/mcp"
	"github.com/Tangerg/lynx/app/runtime/internal/application/models"
	"github.com/Tangerg/lynx/app/runtime/internal/application/ownershiprecovery"
	planapp "github.com/Tangerg/lynx/app/runtime/internal/application/plans"
	"github.com/Tangerg/lynx/app/runtime/internal/application/queries"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/application/schedules"
	"github.com/Tangerg/lynx/app/runtime/internal/application/sessionadmission"
	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/application/taskgroup"
	"github.com/Tangerg/lynx/app/runtime/internal/application/tools"
	"github.com/Tangerg/lynx/app/runtime/internal/application/usage"
	"github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/toolresult"
	"github.com/Tangerg/lynx/app/runtime/internal/idempotency"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/skillauthoring"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/teardown"
)

// Stack is the assembled application surface consumed by delivery. It exposes
// application use cases and notification sources, but owns no resource closers;
// the Host does (§5.3).
type Stack struct {
	Sessions               *sessions.Coordinator
	MCP                    *mcpapp.Coordinator
	Approvals              *approvals.Coordinator
	Models                 *models.Coordinator
	Tools                  *tools.Coordinator
	Codebase               *codebase.Coordinator
	Queries                *queries.Coordinator
	Usage                  *usage.Reporter
	Feedback               *feedbackapp.Recorder
	WorkspaceFiles         *workspace.Files
	WorkspaceVCS           *workspace.VCS
	WorkspaceDiscovery     *workspace.Discovery
	WorkspaceKnowledge     *workspace.Knowledge
	WorkspaceSkills        *workspace.Skills
	WorkspaceHooks         *workspace.Hooks
	WorkspaceWatch         *workspace.GitWatch
	WorkspaceAuthoredWatch *workspace.AuthoredWatch
	Schedules              *schedules.Coordinator
	Goals                  *goals.Driver
	// AgentMemory is the HITL review use-case coordinator over the agent's
	// self-maintained memory (agentMemory.*). It may hold a disabled store, so
	// Delivery can truthfully negotiate the capability without a domain-port leak.
	AgentMemory       *agentmemoryapp.Coordinator
	Runs              *runs.Coordinator
	OwnershipRecovery *ownershiprecovery.Coordinator
	// FileChanges carries committed workspace mutation scopes to protocol
	// subscribers without exposing a wire event to the producer.
	FileChanges NotificationSource[workspace.FileChangeNotice]
	// Invalidations bridges committed read-model changes to the delivery hub,
	// which alone names each wire topic. Producers are the use cases that own the
	// mutation; Bootstrap only connects their shared application vocabulary.
	Invalidations       NotificationSource[invalidation.Notice]
	PublishInvalidation invalidation.Publish
	ScheduleFiring      *schedules.Firing
	IdempotencyStore    idempotency.Store
	GitAvailable        bool
	PlanEnabled         bool
}

type toolEnvironmentBuilder func(
	context.Context,
	Config,
	*approvals.RuntimePolicy,
	mcpEnvironment,
	*agentmemoryapp.Searcher,
	*schedules.Coordinator,
	*goals.Reader,
	*goals.OutcomeReporter,
	*planapp.Coordinator,
	*skillauthoring.Store,
	builtin.SkillProposalSubmitter,
) (toolEnvironment, error)

// Assembly owns configuration resources before construction begins.
type Assembly struct {
	mu         sync.Mutex
	cfg        Config
	buildTools toolEnvironmentBuilder
	lifetime   *hostLifetime
	started    bool
}

// NewAssembly acquires cfg.Resources and returns a single-use Host builder.
func NewAssembly(cfg Config) *Assembly {
	return newAssembly(cfg, buildToolEnvironment)
}

func newAssembly(cfg Config, buildTools toolEnvironmentBuilder) *Assembly {
	return &Assembly{
		cfg:        cfg,
		buildTools: buildTools,
		lifetime: &hostLifetime{
			hostResources: shutdownResources(cfg.Resources),
		},
	}
}

// BuildAssembly constructs and returns a complete Host. On failure it performs
// one rollback attempt and returns nil; CloseAssembly retains any unfinished
// rollback for a later caller-owned attempt.
func BuildAssembly(ctx context.Context, a *Assembly) (*Host, error) {
	if a == nil {
		return nil, errors.New("runtime: nil Assembly")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.started {
		return nil, errors.New("runtime: BuildAssembly called more than once")
	}
	if a.lifetime == nil || a.buildTools == nil {
		return nil, errors.New("runtime: uninitialized Assembly")
	}
	a.started = true
	host, err := buildAssembly(ctx, a)
	if err != nil {
		if rollbackErr := closeHostLifetime(a.lifetime); rollbackErr != nil {
			err = errors.Join(err, fmt.Errorf("runtime: rollback assembly: %w", rollbackErr))
		}
		return nil, err
	}
	a.lifetime = nil
	return host, nil
}

// CloseAssembly releases resources when BuildAssembly has not run, completes
// rollback after a failed build, and is a no-op after ownership transfers to a
// successful Host.
func CloseAssembly(a *Assembly) error {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	// Closing an unstarted Assembly consumes its single use. Otherwise a later
	// BuildAssembly could construct a Host over resources already released.
	a.started = true
	return closeHostLifetime(a.lifetime)
}

func buildAssembly(ctx context.Context, a *Assembly) (*Host, error) {
	cfg := a.cfg
	buildTools := a.buildTools
	lifetime := a.lifetime
	if err := validateAssemblyConfig(cfg); err != nil {
		return nil, err
	}
	// Offloads are staged before their ordered transcript event commits so a
	// following model round can read them immediately. A process crash may leave
	// that short-lived stage behind; startup is the only point with no live tool
	// calls, so reconcile it before constructing the engine.
	if cfg.ToolResultStore != nil {
		if _, err := cfg.ToolResultStore.PurgeUnbound(ctx); err != nil {
			return nil, fmt.Errorf("runtime: reconcile staged tool results: %w", err)
		}
	}

	conversationServices, err := buildConversationEnvironment(
		cfg.ConversationStore,
		persistence.NewConversationCompactions(
			cfg.ConversationStore,
			cfg.RunStore,
			persistence.Transactor(cfg.Transactor),
		),
	)
	if err != nil {
		return nil, err
	}

	modelServices, err := buildModelEnvironment(ctx, cfg)
	if err != nil {
		return nil, err
	}

	// Tool environment: assembled outside the core (constructs the code-intel /
	// exec / MCP / A2A capabilities + the resolver) and injected, so the engine
	// core builds no capability. ctx flows so a slow MCP/A2A dial can be
	// canceled during startup.
	// Permission policy is built early because Plan-mode tools and the execution gate
	// share its narrow session-mode views. The policy owns no Agent execution
	// state: Plan-mode persistence remains a runtime/session concern.
	// One bridge carries every committed change a client can fold — sessions, runs,
	// interrupts, goals, state, schedules, and settings — from the use case that
	// committed it to the delivery hub that names its topic. It is one channel
	// rather than one per resource because the
	// producers publish the same shape (a resource plus the ids that moved), and the
	// wire vocabulary belongs to delivery either way.
	applicationInvalidations := &notification.Relay[invalidation.Notice]{}
	approvalPolicy, err := approvals.NewRuntimePolicy(
		cfg.ApprovalMode, cfg.ApprovalRuleStore, cfg.PermissionModeStore, applicationInvalidations.Publish,
	)
	if err != nil {
		return nil, fmt.Errorf("runtime: approval policy: %w", err)
	}
	// Goal reads and terminal outcome reporting cross into the tool environment
	// before the loop driver can be constructed. They remain separate application
	// boundaries over the same change-publishing store.
	goalStore := goals.WithInvalidations(cfg.GoalStore, applicationInvalidations.Publish)
	goalReader := goals.NewReader(goalStore)
	goalReporter := goals.NewOutcomeReporter(goalStore)
	planCoordinator := planapp.New(planapp.Dependencies{
		Store: cfg.PlanStore, Now: time.Now, Invalidations: applicationInvalidations.Publish,
	})

	mcpConnectionSettings, err := buildMCPEnvironment(ctx, cfg.MCPRegistry)
	if err != nil {
		return nil, err
	}

	scheduleCoordinator := schedules.New(schedules.Dependencies{
		Store:         cfg.ScheduleStore,
		Paths:         workspacepath.Resolver{},
		Invalidations: applicationInvalidations.Publish,
	})
	workspaceScope := workspace.NewScope(cfg.DefaultWorkspacePath, cfg.UserHome, workspacepath.Resolver{})
	agentMemoryCoordinator := agentmemoryapp.New(agentmemoryapp.Config{
		Store:         cfg.AgentMemoryStore,
		Roots:         workspaceScope,
		Invalidations: applicationInvalidations.Publish,
	})
	agentMemoryCuration := agentmemoryapp.NewCuration(agentmemoryapp.CurationConfig{
		Store: cfg.AgentMemoryStore, Invalidations: applicationInvalidations.Publish,
	})
	authoredWatcher, err := checkpointstore.NewAuthoredWatcher(cfg.KnowledgeDirectory, cfg.UserHome, cfg.SkillsUserDir)
	if err != nil {
		return nil, fmt.Errorf("runtime: build authored resource watcher: %w", err)
	}
	workspaceAuthoredWatch := workspace.NewAuthoredWatch(workspaceScope, workspacepath.Resolver{}, authoredWatcher)
	workspaceKnowledge := workspace.NewKnowledge(
		workspaceScope, workspacepath.Resolver{}, cfg.KnowledgeStore, workspaceAuthoredWatch, applicationInvalidations.Publish,
	)
	skillStore := skillauthoring.NewStore(cfg.SkillsUserDir, skills.ScopeUser)
	var skillCurator workspace.SkillCurator
	var idleSkillSweeper workspace.IdleSkillSweeper
	if skillStore.Enabled() {
		skillCurator = skillStore
		idleSkillSweeper = skillStore
	}
	workspaceSkills := workspace.NewSkills(
		workspaceScope,
		promptsource.NewWorkspaceSkills(cfg.SkillsUserDir),
		skillCurator,
		skillproposal.NewLibraries(skillStore),
		workspaceAuthoredWatch,
		applicationInvalidations.Publish,
	)
	skillMaintenance := workspace.NewSkillMaintenance(idleSkillSweeper, workspaceAuthoredWatch, applicationInvalidations.Publish)
	toolRuntime, err := buildTools(ctx, cfg, approvalPolicy, mcpConnectionSettings, modelServices.agentMemorySearch, scheduleCoordinator, goalReader, goalReporter, planCoordinator, skillStore, workspaceSkills)
	lifetime.toolResources = slices.Clone(toolRuntime.closers)
	if err != nil {
		return nil, err
	}

	workingContexts := agentexec.NewWorkingContextComposer(agentexec.WorkingContextConfig{
		UserHome: cfg.UserHome, Knowledge: workspaceKnowledge,
		AgentMemory: cfg.AgentMemoryStore, AgentMemorySearch: modelServices.agentMemorySearch,
		Plan: cfg.PlanStore, Hooks: cfg.HooksResolver,
	})
	toolAuthorizer, err := agentexec.NewToolAuthorizer(approvalPolicy)
	if err != nil {
		return nil, fmt.Errorf("runtime: Tool authorizer: %w", err)
	}
	runMaintenance := buildRunMaintenance(
		cfg, conversationServices, toolRuntime.tools.Shells, workspaceSkills,
		skillMaintenance, agentMemoryCuration,
		modelServices.utilityClient, modelServices.liveEmbedder.ResolveMemory,
	)
	interactionConfig := agentexec.InteractionExecutorConfig{
		BuildID: cfg.BuildID, DefaultClient: cfg.ChatClient, ChatResolver: modelServices.chatResolver,
		ImplementationIdentity: cfg.BuildID,
		ConfigurationIdentity:  "lyra.runtime.interaction.v1",
		StreamModelResponses:   true,
		MaxConcurrentToolCalls: 8,
		ToolInterpreter:        toolset.NewInterpreter(planCoordinator),
		ToolPresenter:          toolset.Presenter{},
		ToolAuthorizer:         toolAuthorizer,
		ToolHooks:              workingContexts,
		MCPToolAutoApproved: func(server, tool string) bool {
			return mcpConnectionSettings.policy.ToolAutoApproved(mcpserver.ToolRef{Server: server, Tool: tool})
		},
		Maintenance:    runMaintenance,
		LifecycleHooks: workingContexts,
		Pricing:        cfg.Pricing,
		Provider:       cfg.Provider,
	}
	if toolRuntime.tools.Resolver != nil {
		interactionConfig.ToolResolver = toolRuntime.tools.Resolver
	}
	if cfg.ToolResultStore != nil {
		interactionConfig.ToolResultStore = cfg.ToolResultStore
		interactionConfig.ToolResultThreshold = cfg.ToolResultThreshold
		interactionConfig.ToolResultReaderName = toolname.ReadToolResult
	}
	interactionExecutor, err := agentexec.NewInteractionExecutor(interactionConfig)
	if err != nil {
		return nil, fmt.Errorf("runtime: Interaction executor: %w", err)
	}
	lifetime.executor = interactionExecutor
	toolRegistry := toolset.NewDiagnosticRegistry()

	// File checkpoints (shadow git) enable run-boundary snapshots + file
	// rollback only when git is present + a dir is configured; the same adapter
	// backs the run-segment boundary snapshot and the sessions file restorer.
	workspaceCheckpoints := checkpointstore.NewCheckpoints(cfg.CheckpointDir)

	// Sandbox isolation for a run whose session is marked Isolated: its tools
	// operate in a throwaway copy of the project directory, the shell OS-jailed.
	// Empty dir disables it (an isolated session's run is then refused, fail-
	// closed). Its copies are destroyed on session delete and at shutdown.
	var isolator *isolation.Isolator
	if cfg.SandboxDir != "" {
		isolator, err = isolation.New(cfg.UserHome, cfg.SandboxDir, cfg.SandboxReadOnlyPaths)
		if err != nil {
			return nil, fmt.Errorf("runtime: build isolated workspace manager: %w", err)
		}
		lifetime.toolResources = append(lifetime.toolResources, teardown.New(func(context.Context) error {
			return isolator.Close()
		}))
	}

	fileChanges := &notification.Relay[workspace.FileChangeNotice]{}
	admissionGate := sessionadmission.New(cfg.SessionOwnership)
	sessionStores := persistence.NewSessionStores(persistence.SessionStoresConfig{
		Sessions:            cfg.SessionStore,
		Transcript:          cfg.TranscriptStore,
		Interrupts:          cfg.InterruptStore,
		Runs:                cfg.RunStore,
		ExecutorCheckpoints: cfg.ExecutorCheckpoints,
		History:             conversationServices.messages,
		Plan:                cfg.PlanStore,
		Approvals:           cfg.ApprovalRuleStore,
		ToolResults:         cfg.ToolResultStore,
		ChildRunStarts:      cfg.ChildRunStartStore,
		Goals:               cfg.GoalStore,
		Tx:                  persistence.Transactor(cfg.Transactor),
	})
	modelCapabilities := modelcatalog.Capabilities{}
	modelCoordinator := models.New(models.Config{
		Providers:          cfg.ProviderRegistry,
		Catalog:            modelCapabilities,
		Prober:             modelCapabilities,
		Lister:             modelCapabilities,
		UtilityRoleState:   modelServices.utilityRoleState,
		UtilityValidator:   modelServices.chatResolver,
		UtilityStore:       cfg.UtilityRoleStore,
		EmbeddingRoleState: modelServices.embeddingRoleState,
		EmbeddingValidator: modelServices.embeddingResolver,
		EmbeddingStore:     cfg.EmbeddingRoleStore,
		Invalidations:      applicationInvalidations.Publish,
	})
	sessionDependencies := sessions.Dependencies{
		Sessions:          cfg.SessionStore,
		Interrupts:        cfg.InterruptStore,
		Transcript:        cfg.TranscriptStore,
		Runs:              cfg.RunStore,
		Boundaries:        cfg.PlanStore,
		PlanReplacements:  planCoordinator,
		Snapshots:         sessionStores,
		MaterialSnapshots: sessionStores,
		Writes:            sessionStores,
		Forgetter:         workingContexts,
		ExecutionReleaser: interactionExecutor,
		Paths:             workspacepath.Resolver{},
		DefaultModel:      cfg.Model,
		Checkpoints:       checkpointstore.NewSessionCheckpoints(workspaceCheckpoints),
		Mutations:         cfg.WorkspaceMutationStore,
		Admissions:        admissionGate,
		Invalidations:     applicationInvalidations.Publish,
		Now:               time.Now,
		NewID: func() string {
			return session.IDPrefix + uuid.NewString()
		},
		NewRunID: func() string {
			return runs.NewRunID(uuid.NewString())
		},
		NewItemID: func() string {
			return runs.NewItemID(uuid.NewString())
		},
		NewToolResultID: toolresult.NewID,
	}
	// Set only when present so a nil *Isolator never reaches the coordinator as a
	// non-nil interface (which would defeat its own nil check).
	if isolator != nil {
		sessionDependencies.Sandbox = isolator
	}
	// The shared Goal/session mutation coordinator is created before either
	// lifecycle owner. The Driver is constructed later because it consumes Runs;
	// no Bootstrap proxy or post-construction mutation is needed.
	var goalMutations *goals.SessionMutations
	if cfg.GoalStore != nil {
		goalMutations = goals.NewSessionMutations()
		sessionDependencies.Goals = goalMutations
	}
	sessionCoordinator := sessions.New(sessionDependencies)
	// The Run coordinator owns the Run lifecycle (§20). Its driven persistence
	// adapter receives only Domain/Application-decided Session values; generated
	// title maintenance returns through the Session Application capability.
	runEffectTasks := &taskgroup.Group{}
	lifetime.runEffectTasks = runEffectTasks
	runSegmentEffects := runsegment.New(runsegment.Config{
		Interrupts:          cfg.InterruptStore,
		ResumeClaims:        cfg.InterruptStore,
		Sessions:            cfg.SessionStore,
		SessionTitles:       sessionCoordinator,
		ScheduleFirings:     cfg.ScheduleStore,
		GoalRuns:            cfg.GoalStore,
		Transcript:          cfg.TranscriptStore,
		ItemReplacer:        cfg.TranscriptStore,
		ToolResults:         cfg.ToolResultStore,
		ModelInvocations:    cfg.ModelInvocationStore,
		ToolInvocations:     cfg.ToolInvocationStore,
		Conversation:        conversationServices.store,
		Titles:              sessiontitle.NewGenerator(modelServices.utilityClient),
		State:               cfg.RunStore,
		RunMetrics:          cfg.RunStore,
		ExecutorCheckpoints: cfg.ExecutorCheckpoints,
		ChildRunStarts:      cfg.ChildRunStartStore,
		Tx:                  runsegment.Transactor(cfg.Transactor),
		Checkpoints:         workspaceCheckpoints,
		Tasks:               runEffectTasks,
		PublishFileChanges:  fileChanges.Publish,
	})
	defaultRunModel, err := modelref.New(cfg.Provider, cfg.Model)
	if err != nil {
		return nil, fmt.Errorf("runtime: default Run model selection: %w", err)
	}
	runDependencies := runs.Dependencies{
		RootStarts:                         interactionExecutor,
		Observations:                       interactionExecutor,
		Releases:                           interactionExecutor,
		RootCancellation:                   interactionExecutor,
		Conversation:                       conversationServices.messages,
		Continuation:                       interactionExecutor,
		WaitingRestorer:                    interactionExecutor,
		Steering:                           interactionExecutor,
		RunningSubtreeCanceler:             interactionExecutor,
		WaitingSubtreeCancellationPreparer: interactionExecutor,
		WorkingContexts:                    workingContexts,
		Session: runs.SessionPorts{
			Reader:       sessionCoordinator,
			Creator:      sessionCoordinator,
			ActiveRuns:   sessionCoordinator,
			Interrupts:   sessionCoordinator,
			Terminations: sessionCoordinator,
		},
		Projection: runs.ProjectionPorts{
			Openings:                    runSegmentEffects,
			Checkpoints:                 runSegmentEffects,
			ResumeClaims:                runSegmentEffects,
			Events:                      runSegmentEffects,
			Barriers:                    runSegmentEffects,
			WaitingSubtreeCancellations: runSegmentEffects,
			Workspace:                   runSegmentEffects,
			Finalizer:                   runSegmentEffects,
		},
		Runs:                  cfg.RunStore,
		Items:                 cfg.TranscriptStore,
		Admissions:            admissionGate,
		DefaultModelSelection: defaultRunModel,
		Now:                   time.Now,
		NewRunID: func() string {
			return runs.NewRunID(uuid.NewString())
		},
		NewSegmentID: func() string {
			return runs.NewSegmentID(uuid.NewString())
		},
		Invalidations: applicationInvalidations.Publish,
	}
	// Set only when present so a nil *Isolator never reaches the coordinator as a
	// non-nil interface (which would defeat its own nil check).
	if isolator != nil {
		runDependencies.Isolation = isolator
	}
	runCoordinator := runs.NewCoordinator(runDependencies)
	lifetime.runCoordinator = runCoordinator
	scheduleFiring := schedules.NewFiring(
		cfg.ScheduleStore,
		schedules.NewRunLauncher(runCoordinator, cfg.DefaultWorkspacePath),
		applicationInvalidations.Publish,
	)

	approvalCoordinator := approvals.New(approvalPolicy, cfg.SessionStore)

	toolCoordinator := tools.New(toolRegistry, workspaceScope)

	mcpCoordinator := mcpapp.New(mcpapp.Config{
		Registry:            cfg.MCPRegistry,
		StatusReader:        toolRuntime.mcp,
		ToolCatalog:         toolRuntime.mcp,
		ConnectionControl:   toolRuntime.mcp,
		ConnectionLifecycle: toolRuntime.mcp,
		Policy:              mcpConnectionSettings.policy,
		Invalidations:       applicationInvalidations.Publish,
	})
	lifetime.mcpCoordinator = mcpCoordinator

	// Goal mode: the autonomous-execution loop driver over the run coordinator.
	// nil store → nil driver → goals.* report capability_not_negotiated.
	var goalDriver *goals.Driver
	if cfg.GoalStore != nil {
		goalDriver = goals.NewDriver(
			goalStore,
			runCoordinator,
			cfg.SessionStore,
			goalMutations,
			cfg.GoalDriveOwnership,
			builtin.RunInstructions,
		)
		lifetime.goalDriver = goalDriver
		// create_goal is the only Goal tool that needs the Driver. Inject the
		// generic tool after Runs and the Driver exist. This must precede Run
		// recovery because it is part of the exact Deployment configuration used
		// to validate a durable executor checkpoint.
		createGoalTool, err := builtin.NewCreate(goalDriver)
		if err != nil {
			return nil, fmt.Errorf("runtime: build create_goal: %w", err)
		}
		if toolRuntime.tools.Resolver != nil {
			toolRuntime.tools.Resolver.UseCreateGoalTool(createGoalTool)
		}
	}

	recoveryPersistence, err := runrecovery.New(runrecovery.Config{
		Sessions:            cfg.SessionStore,
		Runs:                cfg.RunStore,
		Interrupts:          cfg.InterruptStore,
		Transcript:          cfg.TranscriptStore,
		Messages:            conversationServices.store,
		GoalRuns:            cfg.GoalStore,
		ExecutorCheckpoints: cfg.ExecutorCheckpoints,
		ModelInvocations:    cfg.ModelInvocationStore,
		ToolInvocations:     cfg.ToolInvocationStore,
		ChildRunStarts:      cfg.ChildRunStartStore,
		Tx:                  runrecovery.Transactor(cfg.Transactor),
	})
	if err != nil {
		return nil, fmt.Errorf("runtime: boot recovery persistence: %w", err)
	}
	bootRecovery, err := runs.NewRecovery(
		recoveryPersistence,
		interactionExecutor,
		admissionGate,
		applicationInvalidations.Publish,
	)
	if err != nil {
		return nil, fmt.Errorf("runtime: boot recovery: %w", err)
	}
	var goalRecovery ownershiprecovery.Goals
	if goalDriver != nil {
		goalRecovery = goalDriver
	}
	ownershipRecovery, err := ownershiprecovery.New(bootRecovery, goalRecovery, cfg.RecoveryOwnership)
	if err != nil {
		return nil, fmt.Errorf("runtime: ownership recovery: %w", err)
	}
	if err := ownershipRecovery.ReconcileStartup(ctx); err != nil {
		return nil, fmt.Errorf("runtime: reconcile abandoned ownership: %w", err)
	}
	workspaceFiles := workspace.NewFiles(workspaceScope, checkpointstore.FileBrowser{})
	workspaceVCS := workspace.NewVCS(workspaceScope, checkpointstore.VCS{})
	workspaceDiscovery := workspace.NewDiscovery(
		workspaceScope, sessionCoordinator, promptsource.AgentDocs{}, promptsource.NewWorkspaceRecipes(cfg.RecipesGlobalDir),
	)
	workspaceHooks := workspace.NewHooks(
		workspaceScope, cfg.HooksResolver, cfg.HookTrustStore, applicationInvalidations.Publish,
	)
	workspaceWatch := workspace.NewGitWatch(workspaceScope, checkpointstore.GitWatcher{})
	// The @codebase semantic index is its own use-case coordinator (nil index =
	// disabled); it owns the background reindex task group, closed by the Host.
	codebaseCoordinator := codebase.New(
		modelServices.codebaseIndex, workspaceScope, applicationInvalidations.Publish,
	)
	lifetime.codebaseCoordinator = codebaseCoordinator
	host := &Host{
		Stack: Stack{
			Sessions:            sessionCoordinator,
			MCP:                 mcpCoordinator,
			Approvals:           approvalCoordinator,
			Models:              modelCoordinator,
			Tools:               toolCoordinator,
			Codebase:            codebaseCoordinator,
			Runs:                runCoordinator,
			OwnershipRecovery:   ownershipRecovery,
			FileChanges:         fileChanges,
			Invalidations:       applicationInvalidations,
			PublishInvalidation: applicationInvalidations.Publish,
			ScheduleFiring:      scheduleFiring,
			IdempotencyStore:    cfg.IdempotencyStore,
			Queries: queries.New(queries.Dependencies{
				Transcript: cfg.TranscriptStore,
				Interrupts: cfg.InterruptStore,
				Runs:       cfg.RunStore,
				Sessions:   cfg.SessionStore,
				Plan:       cfg.PlanStore,
			}),
			Usage: usage.New(usage.Dependencies{
				Runs: cfg.RunStore, Sessions: cfg.SessionStore,
			}),
			Feedback:               feedbackapp.New(cfg.FeedbackStore),
			WorkspaceFiles:         workspaceFiles,
			WorkspaceVCS:           workspaceVCS,
			WorkspaceDiscovery:     workspaceDiscovery,
			WorkspaceKnowledge:     workspaceKnowledge,
			WorkspaceSkills:        workspaceSkills,
			WorkspaceHooks:         workspaceHooks,
			WorkspaceWatch:         workspaceWatch,
			WorkspaceAuthoredWatch: workspaceAuthoredWatch,
			Schedules:              scheduleCoordinator,
			Goals:                  goalDriver,
			AgentMemory:            agentMemoryCoordinator,
			GitAvailable:           checkpointstore.GitAvailable(),
			PlanEnabled:            cfg.PlanStore != nil,
		},
		lifetime: lifetime,
	}
	return host, nil
}

func validateAssemblyConfig(cfg Config) error {
	if cfg.UserHome == "" {
		return errors.New("runtime: UserHome is required")
	}
	if !filepath.IsAbs(cfg.UserHome) {
		return errors.New("runtime: UserHome must be absolute")
	}
	if cfg.DefaultWorkspacePath == "" {
		return errors.New("runtime: DefaultWorkspacePath is required")
	}
	if !filepath.IsAbs(cfg.DefaultWorkspacePath) {
		return errors.New("runtime: DefaultWorkspacePath must be absolute")
	}
	if cfg.KnowledgeDirectory == "" {
		return errors.New("runtime: KnowledgeDirectory is required")
	}
	for _, path := range []struct {
		name  string
		value string
	}{
		{name: "SkillsUserDir", value: cfg.SkillsUserDir},
		{name: "SandboxDir", value: cfg.SandboxDir},
		{name: "RecipesGlobalDir", value: cfg.RecipesGlobalDir},
		{name: "CheckpointDir", value: cfg.CheckpointDir},
		{name: "KnowledgeDirectory", value: cfg.KnowledgeDirectory},
	} {
		if path.value != "" && !filepath.IsAbs(path.value) {
			return fmt.Errorf("runtime: %s must be absolute when set", path.name)
		}
	}
	for i, path := range cfg.SandboxReadOnlyPaths {
		if path != "" && !filepath.IsAbs(path) {
			return fmt.Errorf("runtime: SandboxReadOnlyPaths[%d] must be absolute when set", i)
		}
	}
	if cfg.ChatClient == nil {
		return errors.New("runtime: ChatClient is required")
	}
	if cfg.BuildID == "" {
		return errors.New("runtime: BuildID is required")
	}
	if cfg.ConversationStore == nil {
		return errors.New("runtime: ConversationStore is required")
	}
	if cfg.ProviderRegistry == nil {
		return errors.New("runtime: ProviderRegistry is required")
	}
	if cfg.MCPRegistry == nil {
		return errors.New("runtime: MCPRegistry is required")
	}
	if cfg.MCPOAuthSessions == nil {
		return errors.New("runtime: MCPOAuthSessions is required")
	}
	if cfg.SessionStore == nil {
		return errors.New("runtime: SessionStore is required")
	}
	if cfg.InterruptStore == nil {
		return errors.New("runtime: InterruptStore is required")
	}
	if cfg.TranscriptStore == nil {
		return errors.New("runtime: TranscriptStore is required")
	}
	if cfg.FeedbackStore == nil {
		return errors.New("runtime: FeedbackStore is required")
	}
	if cfg.RunStore == nil {
		return errors.New("runtime: RunStore is required")
	}
	if cfg.ExecutorCheckpoints == nil {
		return errors.New("runtime: ExecutorCheckpoints is required")
	}
	if cfg.ModelInvocationStore == nil {
		return errors.New("runtime: ModelInvocationStore is required")
	}
	if cfg.ToolInvocationStore == nil {
		return errors.New("runtime: ToolInvocationStore is required")
	}
	if cfg.ChildRunStartStore == nil {
		return errors.New("runtime: ChildRunStartStore is required")
	}
	if cfg.Transactor == nil {
		return errors.New("runtime: Transactor is required")
	}
	return nil
}
