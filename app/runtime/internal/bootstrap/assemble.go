package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/isolation"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/modelcatalog"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/persistence"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/promptsource"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/runrecovery"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/runsegment"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/sessiontitle"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/builtin"
	checkpointstore "github.com/Tangerg/lynx/app/runtime/internal/adapter/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/workspacepath"
	"github.com/Tangerg/lynx/app/runtime/internal/application/approvals"
	feedbackapp "github.com/Tangerg/lynx/app/runtime/internal/application/feedback"
	"github.com/Tangerg/lynx/app/runtime/internal/application/goals"
	mcpapp "github.com/Tangerg/lynx/app/runtime/internal/application/mcp"
	"github.com/Tangerg/lynx/app/runtime/internal/application/models"
	"github.com/Tangerg/lynx/app/runtime/internal/application/ownershiprecovery"
	"github.com/Tangerg/lynx/app/runtime/internal/application/queries"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/application/schedules"
	"github.com/Tangerg/lynx/app/runtime/internal/application/sessionadmission"
	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/application/taskgroup"
	"github.com/Tangerg/lynx/app/runtime/internal/application/tools"
	"github.com/Tangerg/lynx/app/runtime/internal/application/usage"
	"github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/server"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/toolresult"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/teardown"
)

// Assembly owns configuration resources before construction begins.
type Assembly struct {
	mu         sync.Mutex
	cfg        Config
	buildTools toolEnvironmentBuilder
	lifetime   *hostLifetime
	started    bool
}

// NewAssembly acquires cfg.Resources and returns a single-use Host builder.
func NewAssembly(lifetime context.Context, cfg Config) *Assembly {
	return newAssembly(lifetime, cfg, buildToolEnvironment)
}

func newAssembly(
	lifetime context.Context,
	cfg Config,
	buildTools toolEnvironmentBuilder,
) *Assembly {
	return &Assembly{
		cfg:        cfg,
		buildTools: buildTools,
		lifetime: &hostLifetime{
			context:       lifetime,
			hostResources: terminalResources(cfg.Resources),
		},
	}
}

// BuildAssembly constructs and returns a complete Host. On failure it begins a
// bounded rollback and returns nil. The Host-owned shutdown generation keeps
// joining components and the terminal resource Sequence after caller timeout;
// CloseAssembly joins it or starts a new attempt after a settled component
// error.
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
	if a.lifetime.context == nil {
		return nil, errors.New("runtime: Assembly lifetime is required")
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
	if err := validateAssemblyConfig(a.cfg); err != nil {
		return nil, err
	}
	// Offloads are staged before their ordered transcript event commits so a
	// following model round can read them immediately. A process crash may leave
	// that short-lived stage behind; startup is the only point with no live tool
	// calls, so reconcile it before constructing the engine.
	if a.cfg.ToolResultStore != nil {
		if _, err := a.cfg.ToolResultStore.PurgeUnbound(ctx); err != nil {
			return nil, fmt.Errorf("runtime: reconcile staged tool results: %w", err)
		}
	}
	policy, err := buildPolicyComposition(ctx, a.cfg)
	if err != nil {
		return nil, err
	}
	workspaceServices, err := buildWorkspaceComposition(a.cfg, policy.invalidations.Publish)
	if err != nil {
		return nil, err
	}
	execution, err := buildExecutionComposition(
		ctx,
		a.cfg,
		a.lifetime,
		a.buildTools,
		policy,
		workspaceServices,
	)
	if err != nil {
		return nil, err
	}
	return buildAssemblyCore(ctx, a.cfg, a.lifetime, policy, workspaceServices, execution)
}

// buildAssemblyCore composes the Session/Run lifecycle from three complete
// feature capsules. No intermediate locator is published to Delivery.
func buildAssemblyCore(
	ctx context.Context,
	cfg Config,
	lifetime *hostLifetime,
	policy policyComposition,
	workspaceServices workspaceComposition,
	execution executionComposition,
) (*Host, error) {
	var err error

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
		lifetime.toolResources = append(lifetime.toolResources, teardown.Terminal(func(context.Context) error {
			return isolator.Close()
		}))
	}

	fileChanges := newNotificationRelay[workspace.FileChangeNotice]()
	admissionGate := sessionadmission.New(cfg.SessionOwnership)
	sessionStores := persistence.NewSessionStores(persistence.SessionStoresConfig{
		Sessions:            cfg.SessionStore,
		Transcript:          cfg.TranscriptStore,
		Interrupts:          cfg.InterruptStore,
		Runs:                cfg.RunStore,
		ExecutorCheckpoints: cfg.ExecutorCheckpoints,
		History:             execution.conversation.messages,
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
		UtilityRoleState:   execution.models.utilityRoleState,
		UtilityValidator:   execution.models.chatResolver,
		UtilityStore:       cfg.UtilityRoleStore,
		EmbeddingRoleState: execution.models.embeddingRoleState,
		EmbeddingValidator: execution.models.embeddingResolver,
		EmbeddingStore:     cfg.EmbeddingRoleStore,
		Invalidations:      policy.invalidations.Publish,
	})
	defaultRunModel, err := runtimeDefaultModelSelection(cfg)
	if err != nil {
		return nil, err
	}
	sessionDependencies := sessions.Dependencies{
		Sessions:              cfg.SessionStore,
		Interrupts:            cfg.InterruptStore,
		Transcript:            cfg.TranscriptStore,
		Runs:                  cfg.RunStore,
		Snapshots:             sessionStores,
		MaterialSnapshots:     sessionStores,
		Writes:                sessionStores,
		Forgetter:             execution.workingContexts,
		ExecutionReleaser:     execution.executor,
		Paths:                 workspacepath.Resolver{},
		DefaultModelSelection: defaultRunModel,
		Checkpoints:           checkpointstore.NewSessionCheckpoints(workspaceServices.checkpoints),
		Admissions:            admissionGate,
		Invalidations:         policy.invalidations.Publish,
		Now:                   time.Now,
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
	if cfg.PlanStore != nil {
		sessionDependencies.Plan = &sessions.PlanServices{
			Boundaries: cfg.PlanStore, Replacements: policy.plans,
		}
	}
	if cfg.WorkspaceMutationStore != nil {
		sessionDependencies.Mutations = cfg.WorkspaceMutationStore
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
	sessionCoordinator, err := sessions.New(sessionDependencies)
	if err != nil {
		return nil, fmt.Errorf("runtime: construct Session coordinator: %w", err)
	}
	// The Run coordinator owns the Run lifecycle (§20). Its driven persistence
	// adapter receives only Domain/Application-decided Session values; generated
	// title maintenance returns through the Session Application capability.
	runEffectTasks := &taskgroup.Group{}
	lifetime.runEffectTasks = runEffectTasks
	runSegmentConfig := runsegment.Config{
		Interrupts:          cfg.InterruptStore,
		ResumeClaims:        cfg.InterruptStore,
		Sessions:            cfg.SessionStore,
		Transcript:          cfg.TranscriptStore,
		ItemReplacer:        cfg.TranscriptStore,
		ToolApprovals:       cfg.TranscriptStore,
		ModelInvocations:    cfg.ModelInvocationStore,
		ToolInvocations:     cfg.ToolInvocationStore,
		Conversation:        execution.conversation.store,
		State:               cfg.RunStore,
		RunProgress:         cfg.RunStore,
		ExecutorCheckpoints: cfg.ExecutorCheckpoints,
		ChildRunStarts:      cfg.ChildRunStartStore,
		Tx:                  runsegment.Transactor(cfg.Transactor),
	}
	if cfg.ScheduleStore != nil {
		runSegmentConfig.ScheduleFirings = cfg.ScheduleStore
	}
	if cfg.GoalStore != nil {
		runSegmentConfig.GoalRuns = cfg.GoalStore
	}
	if cfg.ToolResultStore != nil {
		runSegmentConfig.ToolResults = cfg.ToolResultStore
	}
	runSegmentEffects, err := runsegment.New(runSegmentConfig)
	if err != nil {
		return nil, fmt.Errorf("runtime: construct Run-segment effects: %w", err)
	}
	runFinalizer, err := runsegment.NewFinalizer(runsegment.FinalizerConfig{
		Checkpoints: workspaceServices.checkpoints,
		Titles: &runsegment.TitleMaintenance{
			Sessions:  sessionCoordinator,
			Generator: sessiontitle.NewGenerator(execution.models.utilityClient),
			Tasks:     runEffectTasks,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("runtime: construct Run finalizer: %w", err)
	}
	workspaceNotifier := runsegment.NewWorkspaceNotifier(fileChanges.Publish)
	runDependencies := runs.Dependencies{
		RootStarts:                         execution.executor,
		Observations:                       execution.executor,
		Releases:                           execution.executor,
		RootCancellation:                   execution.executor,
		Conversation:                       execution.conversation.messages,
		Continuation:                       execution.executor,
		WaitingRestorer:                    execution.executor,
		Steering:                           execution.executor,
		RunningSubtreeCanceler:             execution.executor,
		WaitingSubtreeCancellationPreparer: execution.executor,
		WorkingContexts:                    execution.workingContexts,
		Session: runs.SessionPorts{
			Reader:       sessionCoordinator,
			Creator:      sessionCoordinator,
			ActiveRuns:   sessionCoordinator,
			Interrupts:   sessionCoordinator,
			Terminations: sessionCoordinator,
		},
		Projection: runs.ProjectionPorts{
			Openings:                    runSegmentEffects,
			ChildStarts:                 runSegmentEffects,
			Checkpoints:                 runSegmentEffects,
			ResumeClaims:                runSegmentEffects,
			Events:                      runSegmentEffects,
			Barriers:                    runSegmentEffects,
			WaitingSubtreeCancellations: runSegmentEffects,
			Workspace:                   workspaceNotifier,
			Finalizer:                   runFinalizer,
		},
		Runs:       cfg.RunStore,
		Items:      cfg.TranscriptStore,
		Admissions: admissionGate,
		Now:        time.Now,
		NewRunID: func() string {
			return runs.NewRunID(uuid.NewString())
		},
		NewSegmentID: func() string {
			return runs.NewSegmentID(uuid.NewString())
		},
		Invalidations: policy.invalidations.Publish,
	}
	// Set only when present so a nil *Isolator never reaches the coordinator as a
	// non-nil interface (which would defeat its own nil check).
	if isolator != nil {
		runDependencies.Isolation = isolator
	}
	runCoordinator, err := runs.NewCoordinator(runDependencies)
	if err != nil {
		return nil, fmt.Errorf("runtime: construct Run coordinator: %w", err)
	}
	lifetime.runCoordinator = runCoordinator
	scheduleFiring := schedules.NewFiring(
		cfg.ScheduleStore,
		schedules.NewRunLauncher(runCoordinator, cfg.DefaultWorkspacePath),
		policy.invalidations.Publish,
	)

	approvalCoordinator := approvals.New(policy.approvals, cfg.SessionStore)

	toolCoordinator := tools.New(execution.toolRegistry, workspaceServices.scope)

	mcpCoordinator := mcpapp.New(mcpapp.Config{
		Registry:            cfg.MCPRegistry,
		StatusReader:        execution.tools.mcp,
		ToolCatalog:         execution.tools.mcp,
		ConnectionControl:   execution.tools.mcp,
		ConnectionLifecycle: execution.tools.mcp,
		Policy:              policy.mcp.policy,
		Invalidations:       policy.invalidations.Publish,
	})
	lifetime.mcpCoordinator = mcpCoordinator

	// Goal mode: the autonomous-execution loop driver over the run coordinator.
	// nil store → nil driver → goals.* report capability_not_negotiated.
	var goalDriver *goals.Driver
	if cfg.GoalStore != nil {
		goalDriver = goals.NewDriver(
			policy.goals,
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
		if execution.tools.tools.Resolver != nil {
			execution.tools.tools.Resolver.UseCreateGoalTool(createGoalTool)
		}
	}

	recoveryPersistence, err := runrecovery.New(runrecovery.Config{
		Sessions:            cfg.SessionStore,
		Runs:                cfg.RunStore,
		Interrupts:          cfg.InterruptStore,
		Transcript:          cfg.TranscriptStore,
		Messages:            execution.conversation.store,
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
		execution.executor,
		admissionGate,
		policy.invalidations.Publish,
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
	workspaceFiles := workspace.NewFiles(workspaceServices.scope, checkpointstore.FileBrowser{})
	workspaceVCS := workspace.NewVCS(workspaceServices.scope, checkpointstore.VCS{})
	workspaceDiscovery := workspace.NewDiscovery(
		workspaceServices.scope, sessionCoordinator, promptsource.AgentDocs{}, promptsource.NewWorkspaceRecipes(cfg.RecipesGlobalDir),
	)
	workspaceHooks := workspace.NewHooks(
		workspaceServices.scope, cfg.HooksResolver, cfg.HookTrustStore, policy.invalidations.Publish,
	)
	workspaceWatch := workspace.NewGitWatch(
		workspaceServices.scope,
		checkpointstore.NewGitWatcher(lifetime.context),
	)
	host := &Host{
		application: &hostApplication{
			delivery: server.Config{
				Sessions:      sessionCoordinator,
				MCP:           mcpCoordinator,
				Approvals:     approvalCoordinator,
				Models:        modelCoordinator,
				Tools:         toolCoordinator,
				Runs:          runCoordinator,
				FileChanges:   fileChanges.Observe,
				Invalidations: policy.invalidations.Observe,
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
				WorkspaceKnowledge:     workspaceServices.knowledge,
				WorkspaceSkills:        workspaceServices.skills,
				WorkspaceHooks:         workspaceHooks,
				WorkspaceWatch:         workspaceWatch,
				WorkspaceAuthoredWatch: workspaceServices.authoredWatch,
				Schedules:              policy.schedules,
				ScheduleFiring:         scheduleFiring,
				Goals:                  goalDriver,
				AgentMemory:            workspaceServices.agentMemory,
				GitAvailable:           checkpointstore.GitAvailable(),
				PlanEnabled:            cfg.PlanStore != nil,
			},
			sessions: sessionCoordinator,
			workers: hostWorkers{
				scheduler:     scheduleFiring,
				recovery:      ownershipRecovery,
				invalidations: policy.invalidations.Publish,
			},
			idempotencyStore: cfg.IdempotencyStore,
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
