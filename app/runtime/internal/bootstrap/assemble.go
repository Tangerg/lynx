package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
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
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	checkpointstore "github.com/Tangerg/lynx/app/runtime/internal/adapter/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/workspacepath"
	"github.com/Tangerg/lynx/app/runtime/internal/application/admission"
	agentmemoryapp "github.com/Tangerg/lynx/app/runtime/internal/application/agentmemory"
	"github.com/Tangerg/lynx/app/runtime/internal/application/approvals"
	"github.com/Tangerg/lynx/app/runtime/internal/application/change"
	"github.com/Tangerg/lynx/app/runtime/internal/application/codebase"
	feedbackapp "github.com/Tangerg/lynx/app/runtime/internal/application/feedback"
	"github.com/Tangerg/lynx/app/runtime/internal/application/goals"
	"github.com/Tangerg/lynx/app/runtime/internal/application/integrations"
	"github.com/Tangerg/lynx/app/runtime/internal/application/models"
	"github.com/Tangerg/lynx/app/runtime/internal/application/queries"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/application/schedules"
	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/application/tools"
	"github.com/Tangerg/lynx/app/runtime/internal/application/usage"
	"github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/component/shutdown"
	"github.com/Tangerg/lynx/app/runtime/internal/component/signal"
	"github.com/Tangerg/lynx/app/runtime/internal/component/taskgroup"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/skillauthoring"
	sqlitestore "github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

// Stack is the assembled application: the coordinators + adapters the delivery
// layer drives. It is a pure discovery/delivery aggregate (§5.3) — it owns no
// resource closers; the Host does.
type Stack struct {
	Sessions           *sessions.Coordinator
	Integrations       *integrations.Coordinator
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
	// Coordinator owns the run lifecycle end to end (§8.2/§20): admission, the
	// per-run event journal, the segment pumps, and cancel. Built + owned by the
	// Host (its pumps are joined by Host.Close); the delivery layer drives it as a
	// use-case surface, never constructing it.
	Coordinator *runs.Coordinator
	// FileChanges bridges the run pump's live file-change nudges to the delivery
	// workspace hub (the seam that lets the coordinator be built here rather than
	// inside the delivery Server, §2.5). Delivery installs the consumer via Observe.
	FileChanges *signal.Signal[runs.FileChange]
	// MCPStatus bridges the integrations coordinator's MCP connection transitions
	// to the delivery workspace hub, same seam as FileChanges. Delivery observes it.
	MCPStatus *signal.Signal[integrations.MCPServerStatus]
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
	IdempotencyStore *sqlitestore.IdempotencyStore
	GitAvailable     bool
	TodosEnabled     bool
}

// Host owns the assembled application tier and its process-level close order
// (§13.2). The Stack is a pure discovery/delivery aggregate (§5.3); the Host holds
// the process resources, so delivery reaches coordinators through host.Stack while
// the composition root drives shutdown through Close.
type Host struct {
	Stack Stack

	// lifetime owns the immutable shutdown graph shared by every Host copy.
	lifetime *hostLifetime
}

// RecoverStartup completes durable work that must be reconciled before any
// delivery adapter starts accepting requests. Keeping it as a composition-root
// function, rather than a Host method, keeps Host's public surface limited to
// process lifetime ownership.
func RecoverStartup(ctx context.Context, stack Stack) error {
	if stack.Sessions == nil {
		return errors.New("runtime: sessions coordinator is unavailable for startup recovery")
	}
	return stack.Sessions.RecoverWorkspaceMutations(ctx)
}

type hostLifetime struct {
	closeMu         sync.Mutex
	stopping        bool
	closed          bool
	shutdownTimeout time.Duration

	goals        shutdownComponent
	integrations shutdownComponent
	codebase     shutdownComponent
	coordinator  shutdownComponent
	dispatcher   shutdownComponent
	effectsTasks shutdownComponent
	toolClosers  []ShutdownResource
	resources    []ShutdownResource
}

type shutdownComponent interface {
	BeginShutdown()
	AwaitShutdown(context.Context) error
}

const hostShutdownTimeout = 10 * time.Second

// Close shuts the assembled application tier down in reverse dependency order
// (§10.3). It first broadcasts cancellation to every task-owning component,
// then joins them under one Host-owned deadline. A timeout leaves the graph in
// its stopping phase: a later Close gets a new caller-owned shutdown budget and
// resumes the same in-flight teardown before closing dependent resources.
// Idempotent across Host copies once the graph has fully closed.
func (h *Host) Close() error {
	if h == nil || h.lifetime == nil {
		return nil
	}
	return closeHostLifetime(h.lifetime)
}

func closeHostLifetime(lifetime *hostLifetime) error {
	if lifetime == nil {
		return nil
	}
	lifetime.closeMu.Lock()
	defer lifetime.closeMu.Unlock()
	if lifetime.closed {
		return nil
	}
	components := []shutdownComponent{
		lifetime.goals,
		lifetime.integrations,
		lifetime.codebase,
		lifetime.coordinator,
		lifetime.effectsTasks,
	}
	if !lifetime.stopping {
		lifetime.stopping = true
		for _, component := range components {
			if component != nil {
				component.BeginShutdown()
			}
		}
	}

	timeout := lifetime.shutdownTimeout
	if timeout <= 0 {
		timeout = hostShutdownTimeout
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var errs []error
	for _, component := range components {
		if component != nil {
			errs = append(errs, component.AwaitShutdown(shutdownCtx))
		}
	}
	if componentErr := errors.Join(errs...); componentErr != nil {
		return componentErr
	}

	if lifetime.dispatcher != nil {
		lifetime.dispatcher.BeginShutdown()
		if err := lifetime.dispatcher.AwaitShutdown(shutdownCtx); err != nil {
			return err
		}
	}
	var err error
	lifetime.toolClosers, err = closePendingResources(shutdownCtx, lifetime.toolClosers)
	if err != nil {
		// A closer that failed is still owned by this Host. Keep only those
		// unresolved steps so a later Close retries the real incomplete graph
		// without closing dependencies below an incomplete step.
		return err
	}
	lifetime.resources, err = closePendingResources(shutdownCtx, lifetime.resources)
	if err != nil {
		return err
	}
	lifetime.closed = true
	return nil
}

type toolEnvironmentBuilder func(
	context.Context,
	Config,
	agentexec.Config,
	*approval.RuntimePolicy,
	mcpEnvironment,
	toolset.CodebaseIndex,
	*agentmemory.Searcher,
	*schedules.Coordinator,
	*goals.State,
	*skillauthoring.Store,
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

	// Turn-boundary ports are owned by the dispatcher. The runtime supplies the
	// in-house implementations when the composition root did not inject one.
	// The clientResolver builds a chat client for an explicit (provider, model)
	// from that provider's registry credentials, caching by the credential
	// tuple. A turn uses it to honor a per-run model; the maintenance services
	// below use it to honor the utility-model role.
	providers := cfg.ProviderRegistry
	resolver := modelclient.NewClientResolver(providers)

	utilityRole, err := loadUtilityRole(ctx, cfg.UtilityRoleStore)
	if err != nil {
		return nil, err
	}
	utilityRoleState := models.NewRoleState(utilityRole)
	utilityClient := resolver.UtilityClient(cfg.Engine.ChatClient, utilityRoleState)
	embeddingRole, err := loadEmbeddingRole(ctx, cfg.EmbeddingRoleStore)
	if err != nil {
		return nil, err
	}
	embeddingRoleState := models.NewRoleState(embeddingRole)
	embeddingResolver := modelclient.NewEmbeddingResolver(providers)
	liveEmbedder := modelclient.NewRoleEmbedder(embeddingResolver, embeddingRoleState)
	var codebaseUseCases codebase.Index
	var codebaseToolIndex toolset.CodebaseIndex
	if cfg.CodebaseStore != nil {
		index := codebaseindex.New(cfg.CodebaseStore, liveEmbedder.Resolve, codebaseindexadapter.Source{})
		codebaseUseCases = index
		codebaseToolIndex = index
	}
	// Agent-memory search (memory_search + the extractor's vector backfill) embeds
	// through the same live embedding role as @codebase. The searcher is nil when
	// no memory store is wired; keyword search works without an embedder.
	var memorySearcher *agentmemory.Searcher
	if cfg.AgentMemoryStore != nil {
		memorySearcher = agentmemory.NewSearcher(cfg.AgentMemoryStore, liveEmbedder.ResolveMemory)
	}

	// Tool environment: assembled outside the core (constructs the code-intel /
	// exec / MCP / A2A capabilities + the resolver) and injected, so the engine
	// core builds no capability. ctx flows so a slow MCP/A2A dial can be
	// canceled during startup.
	// Approval stance is built early: the toolset's exit_plan_mode tool needs it
	// (it flips the stance to execute when a plan is approved), and the turn gate
	// reads it per tool call.
	approvalPolicy, err := approval.New(cfg.ApprovalMode, cfg.ApprovalRuleStore)
	if err != nil {
		return nil, fmt.Errorf("runtime: approval policy: %w", err)
	}
	// One bridge carries every committed change a client can fold — sessions, runs,
	// interrupts, goals, state — from the use case that committed it to the delivery
	// hub that names its topic. It is one channel rather than five because the
	// producers publish the same shape (a resource plus the ids that moved), and the
	// wire vocabulary belongs to delivery either way.
	changes := &signal.Signal[change.Notice]{}
	// Goal state crosses into the tool environment before the loop driver can be
	// constructed. It is an application boundary, not a persistence proxy. Wrapping
	// the store here is what makes every goal write — lifecycle command, autonomous
	// turn, update_goal, boot reconcile — publish its own invalidation.
	goalStore := goals.WithChangeNotices(cfg.GoalStore, changes.Publish)
	goalState := goals.NewState(goalStore)

	mcpEnv, err := buildMCPEnvironment(ctx, cfg.MCPRegistry)
	if err != nil {
		return nil, err
	}

	scheduleCoord := schedules.New(schedules.Dependencies{
		Store: cfg.ScheduleStore,
		Paths: workspacepath.Resolver{},
	})
	skillStore := skillauthoring.NewStore(cfg.SkillsGlobalDir)
	built, err := buildTools(ctx, cfg, ecfg, approvalPolicy, mcpEnv, codebaseToolIndex, memorySearcher, scheduleCoord, goalState, skillStore)
	lifetime.toolClosers = slices.Clone(built.closers)
	if err != nil {
		return nil, err
	}
	attachToolEnvironment(&ecfg, built.tools)
	// Per-turn memory recall reuses the same searcher the memory_search tool does.
	if memorySearcher != nil {
		ecfg.MemorySearch = memorySearcher
	}

	// Built after the tool environment so the compactor's live-state reminder can
	// read the same background-shell set the shell tools run over (built.Shells);
	// turnServices is not consumed until the dispatcher config below.
	turnServices := buildTurnServices(cfg, messages, built.tools.Shells, skillStore, utilityClient, liveEmbedder.ResolveMemory)

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
		GoalTurns:           cfg.GoalStore,
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

	turnDispatcher, err := turn.New(turn.Dependencies{
		Engine:              eng,
		Steering:            turnServices.steering,
		Maintenance:         turnServices.maintenance,
		Approval:            approvalPolicy,
		ClientResolver:      resolver,
		Todos:               ecfg.Todos,
		ToolPresenter:       toolset.Presenter{},
		MCPToolAutoApproved: mcpEnv.policy.ToolAutoApproved,
		Hooks:               cfg.HooksResolver,
	})
	if err != nil {
		return nil, fmt.Errorf("runtime: turn dispatcher: %w", err)
	}
	lifetime.dispatcher = turnDispatcher
	home, _ := os.UserHomeDir()
	workspaceContext := workspace.NewContext(cfg.DefaultCwd, home, workspacepath.Resolver{})
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
		isolator = isolation.New(cfg.SandboxDir, cfg.SandboxReadOnlyPaths)
		lifetime.toolClosers = append(lifetime.toolClosers, shutdown.New(func(context.Context) error {
			return isolator.Close()
		}))
	}

	// The run coordinator owns the run lifecycle (§20). It commits durable side
	// effects through the run-segment adapter, whose file-change nudges reach the
	// delivery workspace hub via the notifier the delivery Server observes — the
	// seam that lets the coordinator be constructed here in the Host rather than
	// inside delivery (§11.1/§13.2). It drives the agent turn through the turn
	// Executor (§6.1); the same adapter implements the complete neutral turn-control
	// surface consumed by application/runs.
	fileChanges := &signal.Signal[runs.FileChange]{}
	runExecutor := turn.NewExecutor(turnDispatcher)
	// effectsTasks owns title generation after the synchronous checkpoint
	// boundary; the Host joins accepted title tasks after the pumps.
	effectsTasks := &taskgroup.Group{}
	lifetime.effectsTasks = effectsTasks
	runEffects := runsegment.New(runsegment.Config{
		Interrupts:          cfg.InterruptStore,
		Sessions:            cfg.SessionStore,
		ScheduleFirings:     cfg.ScheduleStore,
		GoalTurns:           cfg.GoalStore,
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
	// mcpStatus bridges the integrations coordinator's MCP reconnect/authorize
	// transitions to the delivery workspace stream the Server observes.
	mcpStatus := &signal.Signal[integrations.MCPServerStatus]{}
	// skillChanges bridges successful skill-library curation and draft promotion
	// to the delivery workspace stream.
	skillChanges := &signal.Signal[struct{}]{}

	admissions := &admission.Gate{}
	sessionStorage := persistence.NewSessionStores(persistence.SessionStoresConfig{
		Sessions:            cfg.SessionStore,
		Transcript:          cfg.TranscriptStore,
		Interrupts:          cfg.InterruptStore,
		Runs:                cfg.RunStore,
		ExecutorCheckpoints: cfg.ExecutorCheckpoints,
		History:             messages.conversation,
		Todos:               cfg.TodoStore,
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
		UtilityValidator:   resolver,
		UtilityStore:       cfg.UtilityRoleStore,
		EmbeddingRoleState: embeddingRoleState,
		EmbeddingValidator: embeddingResolver,
		EmbeddingStore:     cfg.EmbeddingRoleStore,
	})
	sessionDeps := sessions.Dependencies{
		Sessions:     cfg.SessionStore,
		Interrupts:   cfg.InterruptStore,
		Transcript:   cfg.TranscriptStore,
		Runs:         cfg.RunStore,
		Boundaries:   cfg.TodoStore,
		Snapshots:    sessionStorage,
		Writes:       sessionStorage,
		Forgetter:    turnDispatcher,
		Turns:        turn.NewSessionTurnCleanup(turnDispatcher),
		Paths:        workspacepath.Resolver{},
		DefaultModel: cfg.Model,
		Checkpoints:  checkpointstore.NewSessionCheckpoints(checkpoints),
		Mutations:    cfg.WorkspaceMutationStore,
		Admissions:   admissions,
		Changed:      changes.Publish,
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
		Turns:      runExecutor,
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
		schedules.NewRunLauncher(runCoord, cfg.DefaultCwd, scheduleFires.Publish),
	)

	approvalsCoord := approvals.New(approvalPolicy, cfg.SessionStore)

	toolsCoord := tools.New(toolRegistry, workspaceContext)

	integrationsCoord := integrations.New(integrations.Config{
		MCPRegistry:           cfg.MCPRegistry,
		MCPStatusReader:       built.mcp,
		MCPToolCatalog:        built.mcp,
		MCPConnectionCommands: built.mcp,
		MCPRegistryCommands:   built.mcp,
		MCPPolicy:             mcpEnv.policy,
		MCPStatus:             mcpStatus.Publish,
	})
	lifetime.integrations = integrationsCoord

	// Goal mode: the autonomous-execution loop driver over the run coordinator.
	// nil store → nil driver → goals.* report capability_not_negotiated. Reconcile
	// runs before serving so a goal left active by a crashed process degrades to
	// paused rather than silently resuming and burning budget.
	var goalDriver *goals.Driver
	if cfg.GoalStore != nil {
		goalDriver = goals.NewDriverWithMutations(goalStore, runCoord, cfg.SessionStore, goalMutations, agentexec.GoalPrompt)
		lifetime.goals = goalDriver
		if err := goalDriver.Reconcile(ctx); err != nil {
			return nil, fmt.Errorf("runtime: reconcile goals: %w", err)
		}
	}
	// Same discipline for the skill library: leave the ports interface-nil when
	// authoring is disabled (empty skills dir), so the coordinator's nil-gate
	// reports capability_not_negotiated instead of the store's bare disabled error.
	var skillCurator workspace.SkillCurator
	var skillDrafts workspace.SkillDrafts
	if skillStore.Enabled() {
		skillCurator = skillStore
		skillDrafts = skillStore
	}
	workspaceFiles := workspace.NewFiles(workspaceContext, checkpointstore.Reads{})
	workspaceVCS := workspace.NewVCS(workspaceContext, checkpointstore.VCS{})
	workspaceDiscovery := workspace.NewDiscovery(
		workspaceContext, sessionCoord, promptsource.AgentDocs{}, promptsource.NewWorkspaceRecipes(cfg.RecipesGlobalDir),
	)
	workspaceKnowledge := workspace.NewKnowledge(workspaceContext, cfg.KnowledgeStore)
	workspaceSkills := workspace.NewSkills(
		workspaceContext, promptsource.NewWorkspaceSkills(cfg.SkillsGlobalDir), skillCurator, skillDrafts, skillChanges.Publish,
	)
	workspaceHooks := workspace.NewHooks(workspaceContext, cfg.HooksResolver, cfg.HookTrustStore)
	workspaceWatch := workspace.NewGitWatch(workspaceContext, checkpointstore.GitWatcher{})
	// The @codebase semantic index is its own use-case coordinator (nil index =
	// disabled); it owns the background reindex task group, closed by the Host.
	codebaseCoord := codebase.New(codebaseUseCases, workspaceContext)
	lifetime.codebase = codebaseCoord
	agentMemoryCoord := agentmemoryapp.New(agentmemoryapp.Config{
		Store: cfg.AgentMemoryStore,
		Roots: workspaceContext,
	})
	host := &Host{
		Stack: Stack{
			Sessions:         sessionCoord,
			Integrations:     integrationsCoord,
			Approvals:        approvalsCoord,
			Models:           modelsCoord,
			Tools:            toolsCoord,
			Codebase:         codebaseCoord,
			Coordinator:      runCoord,
			FileChanges:      fileChanges,
			MCPStatus:        mcpStatus,
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
				Todos:      cfg.TodoStore,
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
			TodosEnabled:       cfg.TodoStore != nil,
		},
		lifetime: lifetime,
	}
	return host, nil
}

func validateAssemblyConfig(cfg Config) error {
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

func shutdownResources(resources []ShutdownResource) []ShutdownResource {
	steps := make([]ShutdownResource, 0, len(resources))
	for _, resource := range resources {
		if resource == nil {
			continue
		}
		steps = append(steps, shutdown.New(resource.Shutdown))
	}
	return steps
}

func shutdownClosers(closers []func() error) []ShutdownResource {
	steps := make([]ShutdownResource, 0, len(closers))
	for _, closeFn := range closers {
		if closeFn == nil {
			continue
		}
		closeFn := closeFn
		steps = append(steps, shutdown.New(func(context.Context) error {
			return closeFn()
		}))
	}
	return steps
}

func closePendingResources(ctx context.Context, resources []ShutdownResource) ([]ShutdownResource, error) {
	for index := len(resources) - 1; index >= 0; index-- {
		if resource := resources[index]; resource != nil {
			if err := resource.Shutdown(ctx); err != nil {
				// The slice is creation ordered, so the not-yet-run prefix contains
				// dependencies of this failing closer. Do not tear them down beneath
				// an in-flight or failed dependent operation; retain that exact prefix
				// for a later Close instead.
				return slices.Clone(resources[:index+1]), err
			}
		}
	}
	return nil, nil
}
