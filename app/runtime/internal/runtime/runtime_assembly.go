package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/core/model/chat/history"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/maintenance"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/conversation"
	toolsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

// New assembles a Runtime from cfg. Returns an error when a required
// dependency is missing or any internal constructor fails -- engine
// deployment, MCP dial, etc.
func New(ctx context.Context, cfg Config) (*Runtime, error) {
	if cfg.Engine.ChatClient == nil {
		return nil, errors.New("runtime: Engine.ChatClient is required")
	}
	if cfg.TranscriptStore == nil {
		return nil, errors.New("runtime: TranscriptStore is required")
	}

	// The engine config passes through except SessionStore + the microkernel
	// ports. SessionStore: a spawned sub-agent (the `task` delegation) gets its
	// session recorded so the parent->child lineage is durably queryable.
	ecfg := cfg.Engine
	ecfg.SessionStore = newChildSessionStore(cfg.SessionStore)
	// The default provider id -- the engine's pricing fallback for a default /
	// subtask turn that names no provider (so its cost attributes to the right
	// provider rather than an alphabetical catalog guess).
	ecfg.Provider = cfg.Provider

	// Microkernel port wiring: the runtime is the composition root that builds
	// the capability implementations and injects them into the engine core
	// (which depends only on the port interfaces). All share one chat-history
	// store so the engine's chat-history middleware and these ports agree.
	historyStore := cfg.Engine.HistoryStore
	if historyStore == nil {
		historyStore = history.NewInMemoryStore()
		ecfg.HistoryStore = historyStore
	}
	// conv is the message history. The engine gets it ONLY as the
	// turn-end steering sink (engine.InjectUserMessage); the runtime holds it
	// directly for the non-turn history operations (read/seed/count/truncate,
	// for fork / rollback / messages.list) rather than proxying them through the
	// engine. See doc/GREENFIELD_ARCHITECTURE.md.
	conv := conversation.NewMessages(historyStore)

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
	resolver := newClientResolver(providers)

	utilityEnv, err := buildUtilityEnvironment(ctx, cfg, resolver)
	if err != nil {
		return nil, err
	}
	embeddingEnv, err := buildEmbeddingEnvironment(ctx, cfg, providers)
	if err != nil {
		return nil, err
	}

	if ecfg.Steering == nil {
		ecfg.Steering = conv
	}
	wireMaintenancePorts(&ecfg, cfg, historyStore, utilityEnv.resolve)
	// Todo list: same nil-default contract -- honor a pre-injected engine
	// Todos (an external task store), else use the runtime-supplied one.
	if ecfg.Todos == nil {
		ecfg.Todos = cfg.TodoStore
	}

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
		return nil, err
	}

	built, err := buildToolEnvironment(ctx, cfg, ecfg, approvalPolicy, mcpEnv, embeddingEnv.index)
	if err != nil {
		return nil, err
	}
	ecfg.ToolResolver = built.Resolver
	ecfg.Tools = built.Tools
	ecfg.MCP = built.MCP
	ecfg.Closers = built.Closers

	eng, err := kernel.New(ctx, ecfg)
	if err != nil {
		// toolset.Build already dialed MCP/A2A + launched LSP/exec backends into
		// built.Closers; kernel.New didn't take ownership (no engine to Close), so
		// release them here rather than leaking the sessions/processes.
		runClosers(built.Closers)
		return nil, fmt.Errorf("runtime: engine: %w", err)
	}
	// From here the engine owns built.Closers (eng.Close runs them), so a later
	// construction failure tears down via eng.Close.

	// session / interrupt / provider are required and injected by the
	// composition root (cmd/lyra wires sqlite-backed stores; tests wire a
	// sqlite :memory: DB). The runtime keeps no in-memory fallback; there's
	// a single storage backend now.
	sessionSvc := cfg.SessionStore
	interruptStore := cfg.InterruptStore

	turnDispatcher, err := turn.New(turn.Dependencies{
		Engine:         eng,
		Approval:       approvalPolicy,
		ClientResolver: resolver,
		Todos:          ecfg.Todos,
		MCPAutoApprove: mcpEnv.autoApprove,
		Hooks:          cfg.HooksResolver,
	})
	if err != nil {
		_ = eng.Close()
		return nil, fmt.Errorf("runtime: turn dispatcher: %w", err)
	}
	toolRegistry, err := toolsvc.New(eng)
	if err != nil {
		_ = eng.Close()
		return nil, fmt.Errorf("runtime: tool registry: %w", err)
	}

	return &Runtime{
		engine:           eng,
		turns:            turnDispatcher,
		session:          sessionSvc,
		tools:            toolRegistry,
		knowledge:        cfg.Engine.Knowledge,
		approval:         approvalPolicy,
		interrupts:       interruptStore,
		transcript:       cfg.TranscriptStore,
		conversation:     conv,
		providers:        providers,
		mcpRegistry:      cfg.MCPRegistry,
		mcpGating:        mcpEnv.gate,
		defaultProvider:  cfg.Provider,
		defaultModel:     cfg.Model,
		titler:           maintenance.NewTitler(utilityEnv.resolve),
		utility:          utilityEnv.cell,
		resolver:         resolver,
		utilStore:        cfg.UtilityRoleStore,
		hookResolver:     cfg.HooksResolver,
		hookTrust:        cfg.HookTrustStore,
		recipesGlobalDir: cfg.RecipesGlobalDir,
		schedules:        cfg.ScheduleRegistry,
		embeddingCell:    embeddingEnv.cell,
		embeddings:       embeddingEnv.resolver,
		embeddingStore:   cfg.EmbeddingRoleStore,
		codebaseIndex:    embeddingEnv.index,
		transactor:       cfg.Transactor,
	}, nil
}

// runClosers runs capability shutdown hooks best-effort -- used to release a
// half-built tool environment when runtime construction fails before the engine
// (which would otherwise own them) is created.
func runClosers(closers []func() error) {
	for _, closeFn := range closers {
		if closeFn != nil {
			_ = closeFn()
		}
	}
}
