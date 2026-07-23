package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/runsegment"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	checkpointstore "github.com/Tangerg/lynx/app/runtime/internal/adapter/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/workspacepath"
	"github.com/Tangerg/lynx/app/runtime/internal/application/admission"
	agentmemoryapp "github.com/Tangerg/lynx/app/runtime/internal/application/agentmemory"
	"github.com/Tangerg/lynx/app/runtime/internal/application/approvals"
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
	ScheduleFires    *signal.Signal[string]
	ScheduleFiring   *schedules.Firing
	IdempotencyStore *sqlitestore.IdempotencyStore
	GitAvailable     bool
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
	once sync.Once
	err  error

	integrations shutdownComponent
	codebase     shutdownComponent
	coordinator  shutdownComponent
	dispatcher   shutdownDispatcher
	effectsTasks shutdownComponent
	toolClosers  []func() error
	resources    []io.Closer
}

type shutdownComponent interface {
	Close()
}

type shutdownDispatcher interface {
	Close() error
}

// Close shuts the assembled application tier down in reverse dependency order
// (§10.3): the integrations component's post-commit reconcile tasks + the
// codebase reindex tasks first (they depend on the MCP pool / embedding index),
// then the run coordinator (cancel + join every live pump), live turns, the
// run-boundary maintenance tasks, and finally tool capabilities plus injected
// process resources / persistence. Pumps join before maintenance tasks so every
// terminal's boundary work is scheduled. Idempotent across Host copies.
func (h Host) Close() error {
	if h.lifetime == nil {
		return nil
	}
	h.lifetime.once.Do(func() {
		var errs []error
		if h.lifetime.integrations != nil {
			h.lifetime.integrations.Close()
		}
		if h.lifetime.codebase != nil {
			h.lifetime.codebase.Close()
		}
		if h.lifetime.coordinator != nil {
			h.lifetime.coordinator.Close()
		}
		if h.lifetime.dispatcher != nil {
			errs = append(errs, h.lifetime.dispatcher.Close())
		}
		if h.lifetime.effectsTasks != nil {
			h.lifetime.effectsTasks.Close()
		}
		errs = append(
			errs,
			runClosers(h.lifetime.toolClosers),
			closeResources(h.lifetime.resources),
		)
		h.lifetime.err = errors.Join(errs...)
	})
	return h.lifetime.err
}

// Assemble builds the application Host from cfg: it constructs the engine, turn
// dispatcher, tool registry, and the utility/embedding/mcp environments, then builds
// the application coordinators + adapters (run lifecycle, sessions, integrations,
// queries, turn control, workspace, schedules) from those materials, and hands the
// process resources to the Host for shutdown. Returns an error when a required
// dependency is missing or any internal constructor fails — engine deployment,
// MCP dial, etc.
func Assemble(ctx context.Context, cfg Config) (Host, error) {
	return assemble(ctx, cfg, buildToolEnvironment)
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
) (toolset.Built, error)

func assemble(ctx context.Context, cfg Config, buildTools toolEnvironmentBuilder) (_ Host, err error) {
	if cfg.Engine.ChatClient == nil {
		return Host{}, errors.New("runtime: Engine.ChatClient is required")
	}
	if cfg.ProviderRegistry == nil {
		return Host{}, errors.New("runtime: ProviderRegistry is required")
	}
	if cfg.MCPRegistry == nil {
		return Host{}, errors.New("runtime: MCPRegistry is required")
	}
	if cfg.SessionStore == nil {
		return Host{}, errors.New("runtime: SessionStore is required")
	}
	if cfg.InterruptStore == nil {
		return Host{}, errors.New("runtime: InterruptStore is required")
	}
	if cfg.TranscriptStore == nil {
		return Host{}, errors.New("runtime: TranscriptStore is required")
	}
	if cfg.FeedbackStore == nil {
		return Host{}, errors.New("runtime: FeedbackStore is required")
	}
	if cfg.RunStore == nil {
		return Host{}, errors.New("runtime: RunStore is required")
	}
	if cfg.ProcessStore == nil {
		return Host{}, errors.New("runtime: ProcessStore is required")
	}
	if cfg.Transactor == nil {
		return Host{}, errors.New("runtime: Transactor is required")
	}
	// Offloads are staged before their ordered transcript event commits so a
	// following model round can read them immediately. A process crash may leave
	// that short-lived stage behind; startup is the only point with no live tool
	// calls, so reconcile it before constructing the engine.
	if cfg.ToolResultStore != nil {
		if _, err := cfg.ToolResultStore.PurgeUnbound(ctx); err != nil {
			return Host{}, fmt.Errorf("runtime: reconcile staged tool results: %w", err)
		}
	}

	ecfg, messages, err := prepareEngineConfig(cfg)
	if err != nil {
		return Host{}, err
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
		return Host{}, err
	}
	utilityRoleState := models.NewRoleState(utilityRole)
	utilityClient := resolver.UtilityClient(cfg.Engine.ChatClient, utilityRoleState)
	embeddingRole, err := loadEmbeddingRole(ctx, cfg.EmbeddingRoleStore)
	if err != nil {
		return Host{}, err
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
		return Host{}, fmt.Errorf("runtime: approval policy: %w", err)
	}
	// Goal state crosses into the tool environment before the loop driver can be
	// constructed. It is an application boundary, not a persistence proxy.
	goalState := goals.NewState(cfg.GoalStore)

	mcpEnv, err := buildMCPEnvironment(ctx, cfg.MCPRegistry)
	if err != nil {
		return Host{}, err
	}

	scheduleCoord := schedules.New(schedules.Dependencies{
		Store: cfg.ScheduleStore,
		Paths: workspacepath.Resolver{},
	})
	skillStore := skillauthoring.NewStore(cfg.SkillsGlobalDir)
	built, err := buildTools(ctx, cfg, ecfg, approvalPolicy, mcpEnv, codebaseToolIndex, memorySearcher, scheduleCoord, goalState, skillStore)
	if err != nil {
		return Host{}, err
	}
	transferred := false
	defer func() {
		if !transferred {
			err = errors.Join(err, runClosers(built.Closers))
		}
	}()
	attachToolEnvironment(&ecfg, built)
	// Per-turn memory recall reuses the same searcher the memory_search tool does.
	if memorySearcher != nil {
		ecfg.MemorySearch = memorySearcher
	}

	// Built after the tool environment so the compactor's live-state reminder can
	// read the same background-shell set the shell tools run over (built.Shells);
	// turnServices is not consumed until the dispatcher config below.
	turnServices := buildTurnServices(cfg, messages, built.Shells, skillStore, utilityClient, liveEmbedder.ResolveMemory)

	eng, err := agentexec.New(ctx, ecfg)
	if err != nil {
		return Host{}, fmt.Errorf("runtime: engine: %w", err)
	}
	if _, err := cfg.RunStore.ReconcileOrphans(ctx, eng.ResumableProcess); err != nil {
		return Host{}, fmt.Errorf("runtime: reconcile orphan runs: %w", err)
	}

	turnDispatcher, err := turn.New(turn.Dependencies{
		Engine:              eng,
		Steering:            turnServices.steering,
		Compactor:           turnServices.compactor,
		Extractor:           turnServices.extractor,
		Miner:               turnServices.miner,
		SkillCurator:        turnServices.curator,
		Approval:            approvalPolicy,
		ClientResolver:      resolver,
		Todos:               ecfg.Todos,
		MCPToolAutoApproved: mcpEnv.policy.ToolAutoApproved,
		Hooks:               cfg.HooksResolver,
	})
	if err != nil {
		return Host{}, fmt.Errorf("runtime: turn dispatcher: %w", err)
	}
	dispatcherTransferred := false
	defer func() {
		if !dispatcherTransferred {
			err = errors.Join(err, turnDispatcher.Close())
		}
	}()
	toolRegistry, err := toolset.NewRegistry(built.Resolver)
	if err != nil {
		return Host{}, fmt.Errorf("runtime: tool registry: %w", err)
	}

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
	runEffects := runsegment.New(runsegment.Config{
		Interrupts:         cfg.InterruptStore,
		Sessions:           cfg.SessionStore,
		Transcript:         cfg.TranscriptStore,
		ToolResults:        cfg.ToolResultStore,
		Messages:           messages.conversation,
		Titles:             maintenance.NewTitler(utilityClient),
		Processes:          turnDispatcher,
		RunState:           cfg.RunStore,
		Tx:                 runsegment.Transactor(cfg.Transactor),
		Checkpoints:        checkpoints,
		Tasks:              effectsTasks,
		PublishFileChanges: fileChanges.Publish,
	})
	// mcpStatus bridges the integrations coordinator's MCP reconnect/authorize
	// transitions to the delivery workspace stream the Server observes.
	mcpStatus := &signal.Signal[integrations.MCPServerStatus]{}
	// skillChanges bridges successful skill-library curation and draft promotion
	// to the delivery workspace stream.
	skillChanges := &signal.Signal[struct{}]{}

	admissions := &admission.Gate{}
	sessionStorage := persistence.NewSessionStores(persistence.SessionStoresConfig{
		Sessions:    cfg.SessionStore,
		Transcript:  cfg.TranscriptStore,
		Interrupts:  cfg.InterruptStore,
		Runs:        cfg.RunStore,
		Processes:   cfg.ProcessStore,
		History:     messages.conversation,
		Todos:       cfg.TodoStore,
		Approvals:   cfg.ApprovalRuleStore,
		ToolResults: cfg.ToolResultStore,
		Goals:       cfg.GoalStore,
		Tx:          persistence.Transactor(cfg.Transactor),
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
		EmbeddingResolver:  embeddingResolver,
		EmbeddingStore:     cfg.EmbeddingRoleStore,
		DefaultProvider:    cfg.Provider,
		DefaultModel:       cfg.Model,
	})
	sessionDeps := sessions.Dependencies{
		Sessions:    cfg.SessionStore,
		Interrupts:  cfg.InterruptStore,
		Transcript:  cfg.TranscriptStore,
		Snapshots:   sessionStorage,
		Writes:      sessionStorage,
		Forgetter:   turnDispatcher,
		Turns:       turn.NewSessionTurnCleanup(turnDispatcher),
		Paths:       workspacepath.Resolver{},
		Models:      modelsCoord,
		Checkpoints: checkpointstore.NewSessionCheckpoints(checkpoints),
		Mutations:   cfg.WorkspaceMutationStore,
		Admissions:  admissions,
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
		Admissions: admissions,
		Now:        time.Now,
		NewRunID: func() string {
			return runs.NewRunID(uuid.NewString())
		},
		NewSegmentID: func() string {
			return runs.NewSegmentID(uuid.NewString())
		},
	}
	// Set only when present so a nil *Isolator never reaches the coordinator as a
	// non-nil interface (which would defeat its own nil check).
	if isolator != nil {
		runDeps.Isolation = isolator
	}
	runCoord := runs.NewCoordinator(runDeps)
	scheduleFires := &signal.Signal[string]{}
	scheduleFiring := schedules.NewFiring(
		cfg.ScheduleStore,
		schedules.NewRunLauncher(runCoord, cfg.DefaultCwd, scheduleFires.Publish),
	)

	approvalsCoord := approvals.New(approvalPolicy, cfg.SessionStore)

	toolsCoord := tools.New(toolRegistry)

	integrationsCoord := integrations.New(integrations.Config{
		MCPRegistry:           cfg.MCPRegistry,
		MCPStatusReader:       built.MCPStatusReader,
		MCPToolCatalog:        built.MCPToolCatalog,
		MCPConnectionCommands: built.MCPConnectionCommands,
		MCPRegistryCommands:   built.MCPRegistryCommands,
		MCPPolicy:             mcpEnv.policy,
		MCPStatus:             mcpStatus.Publish,
	})

	// Goal mode: the autonomous-execution loop driver over the run coordinator.
	// nil store → nil driver → goals.* report capability_not_negotiated. Reconcile
	// runs before serving so a goal left active by a crashed process degrades to
	// paused rather than silently resuming and burning budget.
	var goalDriver *goals.Driver
	if cfg.GoalStore != nil {
		goalDriver = goals.NewDriverWithMutations(cfg.GoalStore, runCoord, cfg.SessionStore, goalMutations)
		if err := goalDriver.Reconcile(ctx); err != nil {
			return Host{}, fmt.Errorf("runtime: reconcile goals: %w", err)
		}
	}
	toolClosers := slices.Clone(built.Closers)
	if goalDriver != nil {
		toolClosers = append(toolClosers, goalDriver.Close)
	}
	if isolator != nil {
		// Destroy any live isolated working copies on shutdown.
		toolClosers = append(toolClosers, isolator.Close)
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
	home, _ := os.UserHomeDir()
	workspaceContext := workspace.NewContext(cfg.DefaultCwd, home, workspacepath.Resolver{})
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
	agentMemoryCoord := agentmemoryapp.New(agentmemoryapp.Config{
		Store: cfg.AgentMemoryStore,
		Roots: workspaceContext,
	})
	host := Host{
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
			ScheduleFiring:   scheduleFiring,
			IdempotencyStore: cfg.IdempotencyStore,
			Queries: queries.New(queries.Dependencies{
				Transcript: cfg.TranscriptStore,
				Interrupts: cfg.InterruptStore,
			}),
			Usage: usage.New(usage.Dependencies{
				Runs:     cfg.TranscriptStore,
				Sessions: cfg.SessionStore,
				Defaults: modelsCoord,
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
		},
		lifetime: &hostLifetime{
			integrations: integrationsCoord,
			codebase:     codebaseCoord,
			coordinator:  runCoord,
			dispatcher:   turnDispatcher,
			effectsTasks: effectsTasks,
			toolClosers:  toolClosers,
			resources:    slices.Clone(cfg.Resources),
		},
	}
	dispatcherTransferred = true
	transferred = true
	return host, nil
}

// runClosers closes creation-ordered tool resources in reverse.
func runClosers(closers []func() error) error {
	var errs []error
	for index := len(closers) - 1; index >= 0; index-- {
		closeFn := closers[index]
		if closeFn != nil {
			errs = append(errs, closeFn())
		}
	}
	return errors.Join(errs...)
}

func closeResources(resources []io.Closer) error {
	var errs []error
	for index := len(resources) - 1; index >= 0; index-- {
		if resource := resources[index]; resource != nil {
			errs = append(errs, resource.Close())
		}
	}
	return errors.Join(errs...)
}
