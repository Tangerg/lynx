package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/isolation"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/maintenance"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/modelclient"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/runsegment"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	checkpointstore "github.com/Tangerg/lynx/app/runtime/internal/adapter/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/workspacepath"
	"github.com/Tangerg/lynx/app/runtime/internal/application/admission"
	"github.com/Tangerg/lynx/app/runtime/internal/application/approvals"
	"github.com/Tangerg/lynx/app/runtime/internal/application/codebase"
	"github.com/Tangerg/lynx/app/runtime/internal/application/goals"
	"github.com/Tangerg/lynx/app/runtime/internal/application/integrations"
	"github.com/Tangerg/lynx/app/runtime/internal/application/models"
	"github.com/Tangerg/lynx/app/runtime/internal/application/queries"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/application/schedules"
	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/application/tools"
	"github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/component/filechanges"
	"github.com/Tangerg/lynx/app/runtime/internal/component/mcpstatus"
	"github.com/Tangerg/lynx/app/runtime/internal/component/taskgroup"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/skillauthoring"
	sqlitestore "github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

// Stack is the assembled application: the coordinators + adapters the delivery
// layer drives. It is a pure discovery/delivery aggregate (§5.3) — it owns no
// resource closers; the Host does.
type Stack struct {
	Sessions     *sessions.Coordinator
	Integrations *integrations.Coordinator
	Approvals    *approvals.Coordinator
	Models       *models.Coordinator
	Tools        *tools.Coordinator
	Codebase     *codebase.Coordinator
	Queries      *queries.Coordinator
	Workspace    *workspace.Coordinator
	Schedules    *schedules.Coordinator
	Goals        *goals.Driver
	// AgentMemory is the HITL review surface over the agent's self-maintained
	// memory (agentMemory.*). Interface-nil when no memory store is wired.
	AgentMemory agentmemory.Management
	// Coordinator owns the run lifecycle end to end (§8.2/§20): admission, the
	// per-run event journal, the segment pumps, and cancel. Built + owned by the
	// Host (its pumps are joined by Host.Close); the delivery layer drives it as a
	// use-case surface, never constructing it.
	Coordinator *runs.Coordinator
	// FileChanges bridges the run pump's live file-change nudges to the delivery
	// workspace hub (the seam that lets the coordinator be built here rather than
	// inside the delivery Server, §2.5). Delivery installs the consumer via Observe.
	FileChanges *filechanges.Notifier
	// MCPStatus bridges the integrations coordinator's MCP connection transitions
	// to the delivery workspace hub, same seam as FileChanges. Delivery observes it.
	MCPStatus *mcpstatus.Notifier
	// ScheduleFires bridges accepted scheduled-run notifications to the delivery
	// workspace hub. Bootstrap owns the runner; delivery only observes this nudge.
	ScheduleFires    *schedules.FireNotifier
	IdempotencyStore *sqlitestore.IdempotencyStore
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
// dispatcher, tool registry, and the utility/embedding/mcp environments, builds
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
	approval.Policy,
	mcpEnvironment,
	toolset.CodebaseIndex,
	*agentmemory.Searcher,
	*schedules.Coordinator,
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

	utilityEnv, err := buildUtilityEnvironment(ctx, cfg.Engine.ChatClient, cfg.UtilityRoleStore, resolver)
	if err != nil {
		return Host{}, err
	}
	embeddingEnv, err := buildEmbeddingEnvironment(ctx, cfg.EmbeddingRoleStore, cfg.CodebaseStore, providers)
	if err != nil {
		return Host{}, err
	}
	// Agent-memory search (memory_search + the extractor's vector backfill) embeds
	// through the same live embedding role as @codebase. The searcher is nil when
	// no memory store is wired; keyword search works without an embedder.
	memoryEmbed := memoryEmbedder(embeddingEnv.resolveEmbedder)
	var memorySearcher *agentmemory.Searcher
	if cfg.AgentMemoryStore != nil {
		memorySearcher = agentmemory.NewSearcher(cfg.AgentMemoryStore, memoryEmbed)
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

	mcpEnv, err := buildMCPEnvironment(ctx, cfg.MCPRegistry)
	if err != nil {
		return Host{}, err
	}

	scheduleCoord := schedules.New(schedules.Dependencies{
		Registry: cfg.ScheduleRegistry,
		Worker:   cfg.ScheduleRegistry,
		Paths:    workspacepath.Resolver{},
	})
	skillStore := skillauthoring.NewStore(cfg.SkillsGlobalDir)
	built, err := buildTools(ctx, cfg, ecfg, approvalPolicy, mcpEnv, embeddingEnv.index, memorySearcher, scheduleCoord, skillStore)
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
	turnServices := buildTurnServices(cfg, messages, built.Shells, skillStore, utilityEnv.resolve, memoryEmbed)

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
		MCPToolAutoApproved: mcpEnv.toolAutoApproved,
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
	fileChanges := &filechanges.Notifier{}
	runExecutor := turn.NewExecutor(turnDispatcher)
	// effectsTasks owns title generation after the synchronous checkpoint
	// boundary; the Host joins accepted title tasks after the pumps.
	effectsTasks := &taskgroup.Group{}
	runEffects := runsegment.New(runsegment.Config{
		Stores: runSegmentStores{
			interrupts:   cfg.InterruptStore,
			session:      cfg.SessionStore,
			transcript:   cfg.TranscriptStore,
			toolResults:  cfg.ToolResultStore,
			conversation: messages.conversation,
			titler:       maintenance.NewTitler(utilityEnv.resolve),
		},
		Processes:          runSegmentProcesses{dispatcher: turnDispatcher},
		RunState:           cfg.RunStore,
		Tx:                 runsegment.Transactor(cfg.Transactor),
		Checkpoints:        checkpoints,
		Tasks:              effectsTasks,
		PublishFileChanges: fileChanges.Publish,
	})
	// mcpStatus bridges the integrations coordinator's MCP reconnect/authorize
	// transitions to the delivery workspace stream the Server observes.
	mcpStatus := &mcpstatus.Notifier{}

	admissions := &admission.Gate{}
	sessionDeps := sessions.Dependencies{
		Stores: sessionStores{
			sessions:    cfg.SessionStore,
			transcript:  cfg.TranscriptStore,
			interrupts:  cfg.InterruptStore,
			runs:        cfg.RunStore,
			processes:   cfg.ProcessStore,
			history:     messages.conversation,
			todos:       cfg.TodoStore,
			approvals:   cfg.ApprovalRuleStore,
			toolResults: cfg.ToolResultStore,
			// goals puts a deleted/rewound session's goal into the atomic cleanup
			// cascade. cfg.GoalStore is an interface (nil when Goal mode is off), so
			// the write-sets' own nil check skips it — no guard needed here.
			goals:     cfg.GoalStore,
			forgetter: turnDispatcher,
			tx:        cfg.Transactor,
		},
		Turns:       sessionsTurns{dispatcher: turnDispatcher},
		Paths:       workspacepath.Resolver{},
		Checkpoints: sessionCheckpoints{cp: checkpoints},
		Mutations:   cfg.WorkspaceMutationStore,
		Admissions:  admissions,
	}
	// Set only when present so a nil *Isolator never reaches the coordinator as a
	// non-nil interface (which would defeat its own nil check).
	if isolator != nil {
		sessionDeps.Sandbox = isolator
	}
	// The session coordinator serializes write-sets with Goal lifecycle commands,
	// but the Goal driver is built after it (the driver depends on the run
	// coordinator, which depends on the session coordinator). The guard is
	// late-bound before serving to break that construction cycle.
	var goalMutationGuard *goalMutationGuardRef
	if cfg.GoalStore != nil {
		goalMutationGuard = &goalMutationGuardRef{}
		sessionDeps.Goals = goalMutationGuard
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
			return "run_" + uuid.NewString()
		},
		NewSegmentID: func() string {
			return "seg_" + uuid.NewString()
		},
	}
	// Set only when present so a nil *Isolator never reaches the coordinator as a
	// non-nil interface (which would defeat its own nil check).
	if isolator != nil {
		runDeps.Isolation = isolator
	}
	runCoord := runs.NewCoordinator(runDeps)
	scheduleFires := &schedules.FireNotifier{}
	scheduleCoord.BindRunner(schedules.NewRunLauncher(runCoord, cfg.DefaultCwd, scheduleFires.Publish))

	approvalsCoord := approvals.New(approvalPolicy, cfg.SessionStore)

	modelsCoord := models.New(models.Config{
		Providers:         cfg.ProviderRegistry,
		Catalog:           providerCatalog{},
		Prober:            providerProber{},
		Lister:            providerModelLister{},
		UtilityCell:       utilityEnv.cell,
		UtilityValidator:  resolver,
		UtilityStore:      cfg.UtilityRoleStore,
		EmbeddingCell:     embeddingEnv.cell,
		EmbeddingResolver: embeddingEnv.resolver,
		EmbeddingStore:    cfg.EmbeddingRoleStore,
		DefaultProvider:   cfg.Provider,
		DefaultModel:      cfg.Model,
	})

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

	// The @codebase semantic index is its own use-case coordinator (nil index =
	// disabled); it owns the background reindex task group, closed by the Host.
	codebaseCoord := codebase.New(embeddingEnv.index)

	// Goal mode: the autonomous-execution loop driver over the run coordinator.
	// nil store → nil driver → goals.* report capability_not_negotiated. Reconcile
	// runs before serving so a goal left active by a crashed process degrades to
	// paused rather than silently resuming and burning budget.
	var goalDriver *goals.Driver
	if cfg.GoalStore != nil {
		goalDriver = goals.NewDriver(cfg.GoalStore, runCoord, cfg.SessionStore)
		goalMutationGuard.d = goalDriver // late-bind into the session coordinator
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

	// Interface-nil (not a typed-nil holding a nil pointer) when no store is wired,
	// so the server's disabled-capability check fires correctly.
	var agentMemoryMgmt agentmemory.Management
	if cfg.AgentMemoryStore != nil {
		agentMemoryMgmt = cfg.AgentMemoryStore
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
			ScheduleFires:    scheduleFires,
			IdempotencyStore: cfg.IdempotencyStore,
			Queries: queries.New(queries.Dependencies{
				Transcript: cfg.TranscriptStore,
				History:    messages.conversation,
				Interrupts: cfg.InterruptStore,
			}),
			Workspace: workspace.New(workspace.Config{
				Memory:  cfg.Engine.Knowledge,
				Skills:  skillCatalog{globalDir: cfg.SkillsGlobalDir},
				Curator: skillCurator,
				Drafts:  skillDrafts,
				Hooks:   cfg.HooksResolver,
				Trust:   cfg.HookTrustStore,
				Recipes: recipeLister{globalDir: cfg.RecipesGlobalDir},
			}),
			Schedules:   scheduleCoord,
			Goals:       goalDriver,
			AgentMemory: agentMemoryMgmt,
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
