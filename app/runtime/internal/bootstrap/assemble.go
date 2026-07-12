package bootstrap

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/maintenance"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/modelclient"
	checkpointstore "github.com/Tangerg/lynx/app/runtime/internal/adapter/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/application/capabilities"
	"github.com/Tangerg/lynx/app/runtime/internal/application/queries"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/application/schedules"
	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/component/filechanges"
	"github.com/Tangerg/lynx/app/runtime/internal/component/mcpstatus"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	lyraruntime "github.com/Tangerg/lynx/app/runtime/internal/runtime"
)

// Stack is the assembled application: the Runtime facade (the turn/engine
// Executor surface) plus the application coordinators the delivery layer holds
// directly. It grows as facade responsibilities move into coordinators.
type Stack struct {
	Runtime      *lyraruntime.Runtime
	Sessions     *sessions.Coordinator
	Capabilities *capabilities.Coordinator
	Queries      *queries.Coordinator
	TurnControl  *turn.Control
	Workspace    *workspace.Coordinator
	Schedules    *schedules.Coordinator
	// Coordinator owns the run lifecycle end to end (§8.2/§20): admission, the
	// per-run event journal, the segment pumps, and cancel. Built + owned by the
	// Host (its pumps are joined by Host.Close); the delivery layer drives it as a
	// use-case surface, never constructing it.
	Coordinator *runs.Coordinator
	// FileChanges bridges the run pump's live file-change nudges to the delivery
	// workspace hub (the seam that lets the coordinator be built here rather than
	// inside the delivery Server, §2.5). Delivery installs the consumer via Observe.
	FileChanges *filechanges.Notifier
	// MCPStatus bridges the capabilities coordinator's MCP connection transitions
	// to the delivery workspace hub, same seam as FileChanges. Delivery observes it.
	MCPStatus *mcpstatus.Notifier
}

// Host owns the assembled application tier and its process-level close order
// (§13.2). The Stack is a pure discovery/delivery aggregate (§5.3); the Host,
// not the Stack, holds the resource closers, so delivery reaches coordinators
// through host.Stack while the composition root drives shutdown through Close.
type Host struct {
	Stack Stack
}

// Close shuts the assembled application tier down in reverse dependency order
// (§10.3): the capabilities component's post-commit reconcile + reindex tasks
// first (they depend on the engine's MCP pool), then the run coordinator (cancel
// + join every live pump before the engine they drive disappears), then the
// Runtime facade (engine + the injected process resources / persistence).
// Idempotent.
func (h Host) Close() error {
	h.Stack.Capabilities.Close()
	h.Stack.Coordinator.Close()
	return h.Stack.Runtime.Close()
}

// Assemble builds the application Stack from cfg: it constructs the engine, turn
// dispatcher, tool registry, and the utility/embedding/mcp environments, wires
// them into the facade via [lyraruntime.New], and builds the application
// coordinators from the same materials. Returns an error when a required
// dependency is missing or any internal constructor fails — engine deployment,
// MCP dial, etc.
func Assemble(ctx context.Context, cfg lyraruntime.Config) (Host, error) {
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

	ecfg, messages := prepareEngineConfig(cfg)

	// Capability ports are SPIs: the engine consumes interfaces (Steering /
	// Compactor / Extractor; Knowledge above). The runtime supplies the
	// in-house implementations ONLY when the composition root didn't inject one,
	// so an external provider (e.g. a mem0 / HTTP-bridged compactor or knowledge
	// store) can be slotted in by setting the corresponding engine.Config field;
	// the runtime then leaves it untouched. nil -> in-house default.
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

	wireEnginePorts(&ecfg, cfg, messages, utilityEnv.resolve)

	// Tool environment: assembled outside the core (constructs the code-intel /
	// exec / MCP / A2A capabilities + the resolver) and injected, so the engine
	// core builds no capability. ctx flows so a slow MCP/A2A dial can be
	// canceled during startup.
	// Approval stance is built early: the toolset's exit_plan_mode tool needs it
	// (it flips the stance to execute when a plan is approved), and the turn gate
	// reads it per tool call.
	approvalPolicy := approval.New(cfg.ApprovalMode, cfg.ApprovalRuleStore)

	mcpEnv, err := buildMCPEnvironment(ctx, cfg.MCPRegistry)
	if err != nil {
		return Host{}, err
	}

	built, err := buildToolEnvironment(ctx, cfg, ecfg, approvalPolicy, mcpEnv, embeddingEnv.index)
	if err != nil {
		return Host{}, err
	}
	attachToolEnvironment(&ecfg, built)

	eng, err := agentexec.New(ctx, ecfg)
	if err != nil {
		// toolset.Build already dialed MCP/A2A + launched LSP/exec backends into
		// built.Closers; agentexec.New didn't take ownership (no engine to Close), so
		// release them here rather than leaking the sessions/processes.
		return Host{}, errors.Join(fmt.Errorf("runtime: engine: %w", err), runClosers(built.Closers))
	}
	// From here the engine owns built.Closers (eng.Close runs them), so a later
	// construction failure tears down via eng.Close.

	turnDispatcher, err := turn.New(turn.Dependencies{
		Engine:              eng,
		Approval:            approvalPolicy,
		ClientResolver:      resolver,
		Todos:               ecfg.Todos,
		MCPToolAutoApproved: mcpEnv.toolAutoApproved,
		Hooks:               cfg.HooksResolver,
	})
	if err != nil {
		return Host{}, errors.Join(fmt.Errorf("runtime: turn dispatcher: %w", err), eng.Close())
	}
	toolRegistry, err := agentexec.NewToolRegistry(eng)
	if err != nil {
		return Host{}, errors.Join(fmt.Errorf("runtime: tool registry: %w", err), eng.Close())
	}

	rt := lyraruntime.New(lyraruntime.Dependencies{
		Engine:       eng,
		Turns:        turnDispatcher,
		Conversation: messages.conversation,
		Sessions:     cfg.SessionStore,
		Interrupts:   cfg.InterruptStore,
		Transcript:   cfg.TranscriptStore,
		RunState:     cfg.RunStore,
		Transact:     cfg.Transactor,
		Titles:       maintenance.NewTitler(utilityEnv.resolve),
		Resources:    cfg.Resources,
	})

	// File checkpoints (shadow git) enable run-boundary snapshots + file
	// rollback only when git is present + a dir is configured; the same adapter
	// backs the run-segment boundary snapshot and the sessions file restorer.
	checkpoints := checkpointstore.NewCheckpoints(cfg.CheckpointDir)

	// The run coordinator owns the run lifecycle (§20). It commits durable side
	// effects through the run-segment adapter, whose file-change nudges reach the
	// delivery workspace hub via the notifier the delivery Server observes — the
	// seam that lets the coordinator be constructed here in the Host rather than
	// inside delivery (§11.1/§13.2). Built after rt so its executor is the facade.
	fileChanges := &filechanges.Notifier{}
	// The run coordinator drives the agent turn through the turn Executor (§6.1),
	// not the facade — the executor port is the adapter's, the run lifecycle the
	// application's. turnControl is the sibling turn-start adapter delivery drives.
	runExecutor := turn.NewExecutor(turnDispatcher)
	turnControl := turn.NewControl(turnDispatcher, cfg.SessionStore)
	runCoord := runs.NewCoordinator(runExecutor, rt.RunSegmentEffects(checkpoints, fileChanges.Publish), cfg.RunStore)

	// mcpStatus bridges the capabilities coordinator's MCP reconnect/authorize
	// transitions to the delivery workspace stream the Server observes.
	mcpStatus := &mcpstatus.Notifier{}

	sessionCoord := sessions.New(sessions.Dependencies{
		Stores: sessionStores{
			sessions:   cfg.SessionStore,
			transcript: cfg.TranscriptStore,
			interrupts: cfg.InterruptStore,
			runs:       cfg.RunStore,
			history:    messages.conversation,
			forgetter:  turnDispatcher,
			tx:         cfg.Transactor,
		},
		Turns:       sessionsTurns{dispatcher: turnDispatcher},
		Checkpoints: sessionCheckpoints{cp: checkpoints},
		Mutations:   cfg.WorkspaceMutationStore,
	})

	capabilityCoord := capabilities.New(capabilities.Config{
		Approval:          approvalPolicy,
		Tools:             toolRegistry,
		Providers:         cfg.ProviderRegistry,
		Catalog:           providerCatalog{},
		Prober:            providerProber{},
		Sessions:          cfg.SessionStore,
		UtilityCell:       utilityEnv.cell,
		UtilityResolver:   resolver,
		UtilityStore:      cfg.UtilityRoleStore,
		EmbeddingCell:     embeddingEnv.cell,
		EmbeddingResolver: embeddingEnv.resolver,
		EmbeddingStore:    cfg.EmbeddingRoleStore,
		MCPRegistry:       cfg.MCPRegistry,
		MCPLive:           eng,
		MCPPolicy:         mcpEnv.policy,
		Codebase:          embeddingEnv.index,
		MCPStatus:         mcpStatus.Publish,
		DefaultProvider:   cfg.Provider,
		DefaultModel:      cfg.Model,
	})

	return Host{Stack: Stack{
		Runtime:      rt,
		Sessions:     sessionCoord,
		Capabilities: capabilityCoord,
		Coordinator:  runCoord,
		FileChanges:  fileChanges,
		MCPStatus:    mcpStatus,
		TurnControl:  turnControl,
		Queries: queries.New(queries.Dependencies{
			Transcript: cfg.TranscriptStore,
			History:    messages.conversation,
			Interrupts: cfg.InterruptStore,
		}),
		Workspace: workspace.New(workspace.Config{
			Memory:  cfg.Engine.Knowledge,
			Skills:  eng,
			Hooks:   cfg.HooksResolver,
			Trust:   cfg.HookTrustStore,
			Recipes: recipeLister{globalDir: cfg.RecipesGlobalDir},
		}),
		Schedules: schedules.NewCoordinator(cfg.ScheduleRegistry, cfg.ScheduleRegistry),
	}}, nil
}

// runClosers releases a half-built tool environment before the engine can take
// ownership. Every closer runs; the caller joins any cleanup failures with the
// construction error.
func runClosers(closers []func() error) error {
	var errs []error
	for _, closeFn := range closers {
		if closeFn != nil {
			errs = append(errs, closeFn())
		}
	}
	return errors.Join(errs...)
}
