package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
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
	if cfg.ProviderRegistry == nil {
		return nil, errors.New("runtime: ProviderRegistry is required")
	}
	if cfg.MCPRegistry == nil {
		return nil, errors.New("runtime: MCPRegistry is required")
	}
	if cfg.SessionStore == nil {
		return nil, errors.New("runtime: SessionStore is required")
	}
	if cfg.InterruptStore == nil {
		return nil, errors.New("runtime: InterruptStore is required")
	}
	if cfg.TranscriptStore == nil {
		return nil, errors.New("runtime: TranscriptStore is required")
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
	resolver := newClientResolver(providers)

	utilityEnv, err := buildUtilityEnvironment(ctx, cfg.Engine.ChatClient, cfg.UtilityRoleStore, resolver)
	if err != nil {
		return nil, err
	}
	embeddingEnv, err := buildEmbeddingEnvironment(ctx, cfg.EmbeddingRoleStore, cfg.CodebaseStore, providers)
	if err != nil {
		return nil, err
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
		return nil, err
	}

	built, err := buildToolEnvironment(ctx, cfg, ecfg, approvalPolicy, mcpEnv, embeddingEnv.index)
	if err != nil {
		return nil, err
	}
	attachToolEnvironment(&ecfg, built)

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

	return newRuntimeFacade(runtimeFacadeDeps{
		cfg:       cfg,
		engine:    eng,
		turns:     turnDispatcher,
		tools:     toolRegistry,
		approval:  approvalPolicy,
		messages:  messages,
		resolver:  resolver,
		mcp:       mcpEnv,
		utility:   utilityEnv,
		embedding: embeddingEnv,
	}), nil
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
