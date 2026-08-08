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
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	codebaseindexadapter "github.com/Tangerg/lynx/app/runtime/internal/adapter/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/isolation"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/maintenance"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/modelcatalog"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/modelclient"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/persistence"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/promptsource"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/runrecovery"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/runsegment"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/skillproposal"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/skill"
	checkpointstore "github.com/Tangerg/lynx/app/runtime/internal/adapter/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/workspacepath"
	"github.com/Tangerg/lynx/app/runtime/internal/application/admission"
	agentmemoryapp "github.com/Tangerg/lynx/app/runtime/internal/application/agentmemory"
	"github.com/Tangerg/lynx/app/runtime/internal/application/approvals"
	"github.com/Tangerg/lynx/app/runtime/internal/application/change"
	"github.com/Tangerg/lynx/app/runtime/internal/application/codebase"
	feedbackapp "github.com/Tangerg/lynx/app/runtime/internal/application/feedback"
	"github.com/Tangerg/lynx/app/runtime/internal/application/goals"
	mcpapp "github.com/Tangerg/lynx/app/runtime/internal/application/mcp"
	"github.com/Tangerg/lynx/app/runtime/internal/application/models"
	"github.com/Tangerg/lynx/app/runtime/internal/application/queries"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/application/schedules"
	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/application/tools"
	"github.com/Tangerg/lynx/app/runtime/internal/application/usage"
	"github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/component/idempotency"
	"github.com/Tangerg/lynx/app/runtime/internal/component/shutdown"
	"github.com/Tangerg/lynx/app/runtime/internal/component/signal"
	"github.com/Tangerg/lynx/app/runtime/internal/component/taskgroup"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/skillauthoring"
)

// Stack is the assembled application: the coordinators + adapters the delivery
// layer drives. It is a pure discovery/delivery aggregate (§5.3) — it owns no
// resource closers; the Host does.
type Stack struct {
	Sessions           *sessions.Coordinator
	MCP                *mcpapp.Coordinator
	Approvals          *approvals.Coordinator
	Models             *models.Coordinator
	Tools              *tools.Coordinator
	Codebase           *codebase.Coordinator
	Queries            *queries.Coordinator
	Usage              *usage.Reporter
	Feedback           *feedbackapp.Recorder
	WorkspaceFiles     *workspace.Files
	WorkspaceVCS       *workspace.VCS
	WorkspaceDiscovery *workspace.Discovery
	WorkspaceKnowledge *workspace.Knowledge
	WorkspaceSkills    *workspace.Skills
	WorkspaceHooks     *workspace.Hooks
	WorkspaceWatch     *workspace.GitWatch
	Schedules          *schedules.Coordinator
	Goals              *goals.Driver
	// AgentMemory is the HITL review use-case coordinator over the agent's
	// self-maintained memory (agentMemory.*). It may hold a disabled store, so
	// Delivery can truthfully negotiate the capability without a domain-port leak.
	AgentMemory *agentmemoryapp.Coordinator
	Runs        *runs.Coordinator
	// FileChanges carries committed workspace mutation scopes to protocol
	// subscribers without exposing a wire event to the producer.
	FileChanges *signal.Signal[workspace.FileChangeNotice]
	// MCPStatusChanges bridges the MCP coordinator's connection transitions
	// to the delivery workspace hub, same seam as FileChanges. Delivery observes it.
	MCPStatusChanges *signal.Signal[mcpapp.ServerStatus]
	// SkillChanges bridges committed skill-library mutations to the delivery
	// workspace hub. Delivery maps the nudge to a skills.changed event.
	SkillChanges *signal.Signal[struct{}]
	// ScheduleFires bridges accepted scheduled-run notifications to the delivery
	// workspace hub. Bootstrap owns the runner; delivery only observes this nudge.
	ScheduleFires *signal.Signal[string]
	// Changes bridges every committed session / run / interrupt / goal / state change
	// to the delivery hub, which names each one's topic. Same seam as the nudges
	// above; the producers are the use cases that committed the write.
	Changes          *signal.Signal[change.Notice]
	ScheduleFiring   *schedules.Firing
	IdempotencyStore idempotency.Store
	GitAvailable     bool
	PlanEnabled      bool
}

type toolEnvironmentBuilder func(
	context.Context,
	Config,
	agentexec.Config,
	*approvals.RuntimePolicy,
	mcpEnvironment,
	*agentmemoryapp.Searcher,
	*schedules.Coordinator,
	*goals.Reader,
	*goals.OutcomeReporter,
	*skillauthoring.Store,
	skill.ProposalSubmitter,
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
			resources: shutdownResources(cfg.Resources),
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

	ecfg, messages, err := prepareEngineConfig(cfg)
	if err != nil {
		return nil, err
	}

	// Run-boundary ports are owned by the execution controller. The runtime supplies the
	// in-house implementations when the composition root did not inject one.
	// The clientResolver builds a chat client for an explicit (provider, model)
	// from that provider's registry credentials, caching by the credential
	// tuple. A Run uses it to honor a per-Run model; the maintenance services
	// below use it to honor the utility-model role.
	providers := cfg.ProviderRegistry
	chatResolver := modelclient.NewChatResolver(providers)

	utilityRole, err := loadUtilityRole(ctx, cfg.UtilityRoleStore)
	if err != nil {
		return nil, err
	}
	utilityRoleState := models.NewRoleState(utilityRole)
	utilityClient := chatResolver.UtilityClient(cfg.Engine.ChatClient, utilityRoleState)
	embeddingRole, err := loadEmbeddingRole(ctx, cfg.EmbeddingRoleStore)
	if err != nil {
		return nil, err
	}
	embeddingRoleState := models.NewRoleState(embeddingRole)
	embeddingResolver := modelclient.NewEmbeddingResolver(providers)
	liveEmbedder := modelclient.NewRoleEmbedder(embeddingResolver, embeddingRoleState)
	var codebaseUseCases codebase.Index
	if cfg.CodebaseStore != nil {
		index := codebase.NewIndex(cfg.CodebaseStore, liveEmbedder.Resolve, codebaseindexadapter.Source{})
		codebaseUseCases = index
	}
	// Agent-memory search (search_memory + the extractor's vector backfill) embeds
	// through the same live embedding role as @codebase. The searcher is nil when
	// no memory store is wired; keyword search works without an embedder.
	var agentMemorySearcher *agentmemoryapp.Searcher
	if cfg.AgentMemoryStore != nil {
		agentMemorySearcher = agentmemoryapp.NewSearcher(cfg.AgentMemoryStore, liveEmbedder.ResolveMemory)
	}

	// Tool environment: assembled outside the core (constructs the code-intel /
	// exec / MCP / A2A capabilities + the resolver) and injected, so the engine
	// core builds no capability. ctx flows so a slow MCP/A2A dial can be
	// canceled during startup.
	// Permission policy is built early because Plan-mode tools and the execution gate
	// share its narrow session-mode views. The policy owns no Agent execution
	// state: Plan-mode persistence remains a runtime/session concern.
	approvalPolicy, err := approvals.NewRuntimePolicy(cfg.ApprovalMode, cfg.ApprovalRuleStore, cfg.PermissionModeStore)
	if err != nil {
		return nil, fmt.Errorf("runtime: approval policy: %w", err)
	}
	// One bridge carries every committed change a client can fold — sessions, runs,
	// interrupts, goals, state — from the use case that committed it to the delivery
	// hub that names its topic. It is one channel rather than five because the
	// producers publish the same shape (a resource plus the ids that moved), and the
	// wire vocabulary belongs to delivery either way.
	changes := &signal.Signal[change.Notice]{}
	// Goal reads and terminal outcome reporting cross into the tool environment
	// before the loop driver can be constructed. They remain separate application
	// boundaries over the same change-publishing store.
	goalStore := goals.WithChangeNotices(cfg.GoalStore, changes.Publish)
	goalReader := goals.NewReader(goalStore)
	goalReporter := goals.NewOutcomeReporter(goalStore)

	mcpEnv, err := buildMCPEnvironment(ctx, cfg.MCPRegistry)
	if err != nil {
		return nil, err
	}

	scheduleCoord := schedules.New(schedules.Dependencies{
		Store: cfg.ScheduleStore,
		Paths: workspacepath.Resolver{},
	})
	workspaceScope := workspace.NewScope(cfg.DefaultWorkspacePath, cfg.UserHome, workspacepath.Resolver{})
	// One signal covers every committed Skill-library mutation, including
	// proposal submission and review decisions.
	skillChanges := &signal.Signal[struct{}]{}
	skillStore := skillauthoring.NewStore(cfg.SkillsUserDir, skills.ScopeUser)
	var skillCurator workspace.SkillCurator
	if skillStore.Enabled() {
		skillCurator = skillStore
	}
	workspaceSkills := workspace.NewSkills(
		workspaceScope,
		promptsource.NewWorkspaceSkills(cfg.SkillsUserDir),
		skillCurator,
		skillproposal.NewLibraries(skillStore),
		skillChanges.Publish,
	)
	built, err := buildTools(ctx, cfg, ecfg, approvalPolicy, mcpEnv, agentMemorySearcher, scheduleCoord, goalReader, goalReporter, skillStore, workspaceSkills)
	lifetime.toolClosers = slices.Clone(built.closers)
	if err != nil {
		return nil, err
	}
	attachToolEnvironment(&ecfg, built.tools)
	// Per-Run memory recall reuses the same searcher the search_memory tool does.
	if agentMemorySearcher != nil {
		ecfg.AgentMemorySearch = agentMemorySearcher
	}

	// Built after the tool environment so the compactor's live-state reminder can
	// read the same background-shell set the shell tools run over (built.Shells);
	// executionSupport is not consumed until the execution-controller config below.
	executionSupport := buildExecutionSupport(cfg, messages, built.tools.Shells, skillStore, workspaceSkills, utilityClient, liveEmbedder.ResolveMemory)

	eng, err := agentexec.New(ctx, ecfg)
	if err != nil {
		return nil, fmt.Errorf("runtime: engine: %w", err)
	}
	recoveryPersistence, err := runrecovery.New(runrecovery.Config{
		Sessions:            cfg.SessionStore,
		Runs:                cfg.RunStore,
		Interrupts:          cfg.InterruptStore,
		Transcript:          cfg.TranscriptStore,
		Messages:            messages.conversation,
		GoalRuns:            cfg.GoalStore,
		ExecutorCheckpoints: cfg.ExecutorCheckpoints,
		Tx:                  runrecovery.Transactor(cfg.Transactor),
	})
	if err != nil {
		return nil, fmt.Errorf("runtime: boot recovery persistence: %w", err)
	}
	bootRecovery, err := runs.NewRecovery(recoveryPersistence, eng)
	if err != nil {
		return nil, fmt.Errorf("runtime: boot recovery: %w", err)
	}
	if _, err := bootRecovery.Reconcile(ctx); err != nil {
		return nil, fmt.Errorf("runtime: reconcile abandoned Runs: %w", err)
	}

	executionController, err := turn.New(turn.Dependencies{
		Engine:              eng,
		Steering:            executionSupport.steering,
		Maintenance:         executionSupport.maintenance,
		Approval:            approvalPolicy,
		ChatResolver:        chatResolver,
		ToolPresenter:       toolset.Presenter{},
		ToolInterpreter:     toolset.NewInterpreter(cfg.PlanStore),
		MCPToolAutoApproved: mcpEnv.policy.ToolAutoApproved,
		Hooks:               cfg.HooksResolver,
	})
	if err != nil {
		return nil, fmt.Errorf("runtime: execution controller: %w", err)
	}
	lifetime.execution = executionController
	toolRegistry := toolset.NewDiagnosticRegistry()

	// File checkpoints (shadow git) enable run-boundary snapshots + file
	// rollback only when git is present + a dir is configured; the same adapter
	// backs the run-segment boundary snapshot and the sessions file restorer.
	checkpoints := checkpointstore.NewCheckpoints(cfg.CheckpointDir)

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
		lifetime.toolClosers = append(lifetime.toolClosers, shutdown.New(func(context.Context) error {
			return isolator.Close()
		}))
	}

	// The Run coordinator owns the Run lifecycle (§20). It commits durable effects
	// through the Run-segment adapter and publishes file-change nudges through the
	// workspace signal. Agent execution is driven through the same semantic
	// execution-control adapter consumed by application/runs.
	fileChanges := &signal.Signal[workspace.FileChangeNotice]{}
	runExecutor := turn.NewExecutor(executionController)
	// effectsTasks owns title generation after the synchronous checkpoint
	// boundary; the Host joins accepted title tasks after the pumps.
	effectsTasks := &taskgroup.Group{}
	lifetime.effectsTasks = effectsTasks
	runEffects := runsegment.New(runsegment.Config{
		Interrupts:          cfg.InterruptStore,
		Sessions:            cfg.SessionStore,
		ScheduleFirings:     cfg.ScheduleStore,
		GoalRuns:            cfg.GoalStore,
		Transcript:          cfg.TranscriptStore,
		ItemReplacer:        cfg.TranscriptStore,
		ToolResults:         cfg.ToolResultStore,
		Messages:            messages.conversation,
		Titles:              maintenance.NewTitler(utilityClient),
		RunState:            cfg.RunStore,
		ExecutorCheckpoints: cfg.ExecutorCheckpoints,
		Tx:                  runsegment.Transactor(cfg.Transactor),
		Checkpoints:         checkpoints,
		Tasks:               effectsTasks,
		PublishFileChanges:  fileChanges.Publish,
	})
	// mcpStatus bridges the MCP coordinator's reconnect/authorize
	// transitions to the delivery workspace stream the Server observes.
	mcpStatus := &signal.Signal[mcpapp.ServerStatus]{}
	admissions := &admission.Gate{}
	sessionStorage := persistence.NewSessionStores(persistence.SessionStoresConfig{
		Sessions:            cfg.SessionStore,
		Transcript:          cfg.TranscriptStore,
		Interrupts:          cfg.InterruptStore,
		Runs:                cfg.RunStore,
		ExecutorCheckpoints: cfg.ExecutorCheckpoints,
		History:             messages.conversation,
		Plan:                cfg.PlanStore,
		Approvals:           cfg.ApprovalRuleStore,
		ToolResults:         cfg.ToolResultStore,
		Goals:               cfg.GoalStore,
		Tx:                  persistence.Transactor(cfg.Transactor),
	})
	modelCapabilities := modelcatalog.Capabilities{}
	modelsCoord := models.New(models.Config{
		Providers:          cfg.ProviderRegistry,
		Catalog:            modelCapabilities,
		Prober:             modelCapabilities,
		Lister:             modelCapabilities,
		UtilityRoleState:   utilityRoleState,
		UtilityValidator:   chatResolver,
		UtilityStore:       cfg.UtilityRoleStore,
		EmbeddingRoleState: embeddingRoleState,
		EmbeddingValidator: embeddingResolver,
		EmbeddingStore:     cfg.EmbeddingRoleStore,
	})
	sessionDeps := sessions.Dependencies{
		Sessions:         cfg.SessionStore,
		Interrupts:       cfg.InterruptStore,
		Transcript:       cfg.TranscriptStore,
		Runs:             cfg.RunStore,
		Boundaries:       cfg.PlanStore,
		Snapshots:        sessionStorage,
		Writes:           sessionStorage,
		Forgetter:        executionController,
		ExecutionCleanup: turn.NewSessionExecutionCleanup(executionController),
		Paths:            workspacepath.Resolver{},
		DefaultModel:     cfg.Model,
		Checkpoints:      checkpointstore.NewSessionCheckpoints(checkpoints),
		Mutations:        cfg.WorkspaceMutationStore,
		Admissions:       admissions,
		Changed:          changes.Publish,
	}
	// Set only when present so a nil *Isolator never reaches the coordinator as a
	// non-nil interface (which would defeat its own nil check).
	if isolator != nil {
		sessionDeps.Sandbox = isolator
	}
	// The shared Goal/session mutation coordinator is created before either
	// lifecycle owner. The Driver is constructed later because it consumes Runs;
	// no Bootstrap proxy or post-construction mutation is needed.
	var goalMutations *goals.SessionMutations
	if cfg.GoalStore != nil {
		goalMutations = goals.NewSessionMutations()
		sessionDeps.Goals = goalMutations
	}
	sessionCoord := sessions.New(sessionDeps)
	runDeps := runs.Dependencies{
		Segments:   runExecutor,
		Control:    runExecutor,
		Sessions:   sessionCoord,
		Effects:    runEffects,
		Runs:       cfg.RunStore,
		Items:      cfg.TranscriptStore,
		Admissions: admissions,
		Now:        time.Now,
		NewRunID: func() string {
			return runs.NewRunID(uuid.NewString())
		},
		NewSegmentID: func() string {
			return runs.NewSegmentID(uuid.NewString())
		},
		Changed: changes.Publish,
	}
	// Set only when present so a nil *Isolator never reaches the coordinator as a
	// non-nil interface (which would defeat its own nil check).
	if isolator != nil {
		runDeps.Isolation = isolator
	}
	runCoord := runs.NewCoordinator(runDeps)
	lifetime.coordinator = runCoord
	scheduleFires := &signal.Signal[string]{}
	scheduleFiring := schedules.NewFiring(
		cfg.ScheduleStore,
		schedules.NewRunLauncher(runCoord, cfg.DefaultWorkspacePath, scheduleFires.Publish),
	)

	approvalsCoord := approvals.New(approvalPolicy, cfg.SessionStore)

	toolsCoord := tools.New(toolRegistry, workspaceScope)

	mcpCoord := mcpapp.New(mcpapp.Config{
		Registry:            cfg.MCPRegistry,
		StatusReader:        built.mcp,
		ToolCatalog:         built.mcp,
		ConnectionControl:   built.mcp,
		ConnectionLifecycle: built.mcp,
		Policy:              mcpEnv.policy,
		StatusChanged:       mcpStatus.Publish,
	})
	lifetime.mcp = mcpCoord

	// Goal mode: the autonomous-execution loop driver over the run coordinator.
	// nil store → nil driver → goals.* report capability_not_negotiated. Reconcile
	// runs before serving so a goal left active by a crashed process degrades to
	// paused rather than silently resuming and burning budget.
	var goalDriver *goals.Driver
	if cfg.GoalStore != nil {
		goalDriver = goals.NewDriver(goalStore, runCoord, cfg.SessionStore, goalMutations, goal.RunInstructions)
		lifetime.goals = goalDriver
		if err := goalDriver.Reconcile(ctx); err != nil {
			return nil, fmt.Errorf("runtime: reconcile goals: %w", err)
		}
		// create_goal is the only Goal tool that needs the Driver. Inject the
		// generic tool after Runs and the Driver exist instead of leaking either
		// application type into agentexec or introducing a mutable lifecycle proxy.
		createGoalTool, err := goal.NewCreate(goalDriver)
		if err != nil {
			return nil, fmt.Errorf("runtime: build create_goal: %w", err)
		}
		if built.tools.Resolver != nil {
			built.tools.Resolver.UseCreateGoalTool(createGoalTool)
		}
	}
	workspaceFiles := workspace.NewFiles(workspaceScope, checkpointstore.FileBrowser{})
	workspaceVCS := workspace.NewVCS(workspaceScope, checkpointstore.VCS{})
	workspaceDiscovery := workspace.NewDiscovery(
		workspaceScope, sessionCoord, promptsource.AgentDocs{}, promptsource.NewWorkspaceRecipes(cfg.RecipesGlobalDir),
	)
	workspaceKnowledge := workspace.NewKnowledge(workspaceScope, cfg.KnowledgeStore)
	workspaceHooks := workspace.NewHooks(workspaceScope, cfg.HooksResolver, cfg.HookTrustStore)
	workspaceWatch := workspace.NewGitWatch(workspaceScope, checkpointstore.GitWatcher{})
	// The @codebase semantic index is its own use-case coordinator (nil index =
	// disabled); it owns the background reindex task group, closed by the Host.
	codebaseCoord := codebase.New(codebaseUseCases, workspaceScope)
	lifetime.codebase = codebaseCoord
	agentMemoryCoord := agentmemoryapp.New(agentmemoryapp.Config{
		Store: cfg.AgentMemoryStore,
		Roots: workspaceScope,
	})
	host := &Host{
		Stack: Stack{
			Sessions:         sessionCoord,
			MCP:              mcpCoord,
			Approvals:        approvalsCoord,
			Models:           modelsCoord,
			Tools:            toolsCoord,
			Codebase:         codebaseCoord,
			Runs:             runCoord,
			FileChanges:      fileChanges,
			MCPStatusChanges: mcpStatus,
			SkillChanges:     skillChanges,
			ScheduleFires:    scheduleFires,
			Changes:          changes,
			ScheduleFiring:   scheduleFiring,
			IdempotencyStore: cfg.IdempotencyStore,
			Queries: queries.New(queries.Dependencies{
				Transcript: cfg.TranscriptStore,
				Interrupts: cfg.InterruptStore,
				Runs:       cfg.RunStore,
				Sessions:   cfg.SessionStore,
				Plan:       cfg.PlanStore,
			}),
			Usage: usage.New(usage.Dependencies{
				Runs:            cfg.RunStore,
				Sessions:        cfg.SessionStore,
				DefaultProvider: cfg.Provider,
				DefaultModel:    cfg.Model,
			}),
			Feedback:           feedbackapp.New(cfg.FeedbackStore),
			WorkspaceFiles:     workspaceFiles,
			WorkspaceVCS:       workspaceVCS,
			WorkspaceDiscovery: workspaceDiscovery,
			WorkspaceKnowledge: workspaceKnowledge,
			WorkspaceSkills:    workspaceSkills,
			WorkspaceHooks:     workspaceHooks,
			WorkspaceWatch:     workspaceWatch,
			Schedules:          scheduleCoord,
			Goals:              goalDriver,
			AgentMemory:        agentMemoryCoord,
			GitAvailable:       checkpointstore.GitAvailable(),
			PlanEnabled:        cfg.PlanStore != nil,
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
	for _, path := range []struct {
		name  string
		value string
	}{
		{name: "SkillsUserDir", value: cfg.SkillsUserDir},
		{name: "SandboxDir", value: cfg.SandboxDir},
		{name: "RecipesGlobalDir", value: cfg.RecipesGlobalDir},
		{name: "CheckpointDir", value: cfg.CheckpointDir},
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
	if cfg.Engine.ChatClient == nil {
		return errors.New("runtime: Engine.ChatClient is required")
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
	if cfg.Transactor == nil {
		return errors.New("runtime: Transactor is required")
	}
	return nil
}
