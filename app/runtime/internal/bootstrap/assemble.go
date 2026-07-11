package bootstrap

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/maintenance"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/modelclient"
	"github.com/Tangerg/lynx/app/runtime/internal/application/schedules"
	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	toolsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
	lyraruntime "github.com/Tangerg/lynx/app/runtime/internal/runtime"
)

// Stack is the assembled application: the Runtime facade (the turn/engine
// Executor surface) plus the application coordinators the delivery layer holds
// directly. It grows as facade responsibilities move into coordinators.
type Stack struct {
	Runtime   *lyraruntime.Runtime
	Sessions  *sessions.Coordinator
	Workspace *workspace.Coordinator
	Schedules *schedules.Coordinator
}

// Assemble builds the application Stack from cfg: it constructs the engine, turn
// dispatcher, tool registry, and the utility/embedding/mcp environments, wires
// them into the facade via [lyraruntime.New], and builds the application
// coordinators from the same materials. Returns an error when a required
// dependency is missing or any internal constructor fails — engine deployment,
// MCP dial, etc.
func Assemble(ctx context.Context, cfg lyraruntime.Config) (Stack, error) {
	if cfg.Engine.ChatClient == nil {
		return Stack{}, errors.New("runtime: Engine.ChatClient is required")
	}
	if cfg.ProviderRegistry == nil {
		return Stack{}, errors.New("runtime: ProviderRegistry is required")
	}
	if cfg.MCPRegistry == nil {
		return Stack{}, errors.New("runtime: MCPRegistry is required")
	}
	if cfg.SessionStore == nil {
		return Stack{}, errors.New("runtime: SessionStore is required")
	}
	if cfg.InterruptStore == nil {
		return Stack{}, errors.New("runtime: InterruptStore is required")
	}
	if cfg.TranscriptStore == nil {
		return Stack{}, errors.New("runtime: TranscriptStore is required")
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
		return Stack{}, err
	}
	embeddingEnv, err := buildEmbeddingEnvironment(ctx, cfg.EmbeddingRoleStore, cfg.CodebaseStore, providers)
	if err != nil {
		return Stack{}, err
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
		return Stack{}, err
	}

	built, err := buildToolEnvironment(ctx, cfg, ecfg, approvalPolicy, mcpEnv, embeddingEnv.index)
	if err != nil {
		return Stack{}, err
	}
	attachToolEnvironment(&ecfg, built)

	eng, err := kernel.New(ctx, ecfg)
	if err != nil {
		// toolset.Build already dialed MCP/A2A + launched LSP/exec backends into
		// built.Closers; kernel.New didn't take ownership (no engine to Close), so
		// release them here rather than leaking the sessions/processes.
		return Stack{}, errors.Join(fmt.Errorf("runtime: engine: %w", err), runClosers(built.Closers))
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
		return Stack{}, errors.Join(fmt.Errorf("runtime: turn dispatcher: %w", err), eng.Close())
	}
	toolRegistry, err := toolsvc.New(eng)
	if err != nil {
		return Stack{}, errors.Join(fmt.Errorf("runtime: tool registry: %w", err), eng.Close())
	}

	rt := lyraruntime.New(lyraruntime.Dependencies{
		Engine:          eng,
		Turns:           turnDispatcher,
		Tools:           toolRegistry,
		Approval:        approvalPolicy,
		Conversation:    messages.conversation,
		Resolver:        resolver,
		Sessions:        cfg.SessionStore,
		Interrupts:      cfg.InterruptStore,
		Transcript:      cfg.TranscriptStore,
		Providers:       cfg.ProviderRegistry,
		MCPRegistry:     cfg.MCPRegistry,
		MCPPolicy:       mcpEnv.policy,
		DefaultProvider: cfg.Provider,
		DefaultModel:    cfg.Model,
		Titles:          maintenance.NewTitler(utilityEnv.resolve),
		UtilityCell:     utilityEnv.cell,
		UtilityStore:    cfg.UtilityRoleStore,
		EmbeddingCell:   embeddingEnv.cell,
		Embeddings:      embeddingEnv.resolver,
		EmbeddingStore:  cfg.EmbeddingRoleStore,
		Codebase:        embeddingEnv.index,
		Resources:       cfg.Resources,
	})

	sessionCoord := sessions.New(sessions.Dependencies{
		Stores: sessionStores{
			sessions:   cfg.SessionStore,
			transcript: cfg.TranscriptStore,
			interrupts: cfg.InterruptStore,
			history:    messages.conversation,
			forgetter:  turnDispatcher,
			tx:         cfg.Transactor,
		},
		Turns: turnDispatcher,
	})

	return Stack{
		Runtime:  rt,
		Sessions: sessionCoord,
		Workspace: workspace.New(workspace.Config{
			Memory:           cfg.Engine.Knowledge,
			Skills:           eng,
			Hooks:            cfg.HooksResolver,
			Trust:            cfg.HookTrustStore,
			RecipesGlobalDir: cfg.RecipesGlobalDir,
		}),
		Schedules: schedules.NewCoordinator(cfg.ScheduleRegistry, cfg.ScheduleRegistry),
	}, nil
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
