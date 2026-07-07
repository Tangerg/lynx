package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"go.opentelemetry.io/otel/trace"

	adapterhooks "github.com/Tangerg/lynx/app/runtime/internal/adapter/hooks"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	providersvc "github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/llm"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
	lyraruntime "github.com/Tangerg/lynx/app/runtime/internal/runtime"
)

// ensureRuntime lazily builds the runtime bundle. Idempotent —
// safe to call from every RunE entry point.
//
// Building the chat client requires a valid API key, so calling
// this from a no-args help command would falsely demand one. Help
// / version commands therefore don't call ensureRuntime; commands
// that actually talk to the model do.
func (a *App) ensureRuntime(ctx context.Context) error {
	if a.rt != nil {
		return nil
	}
	cfg, err := loadRuntimeConfig()
	if err != nil {
		return err
	}
	// The default client is the configured provider+model — the one a turn
	// runs against when it doesn't pick a model. Per-run model selection
	// builds other clients on demand from the provider registry.
	client, err := llm.BuildClient(llm.ClientSpec{
		Provider: llm.Provider(cfg.Provider),
		Model:    cfg.Model,
		APIKey:   cfg.APIKey,
		BaseURL:  cfg.BaseURL,
	})
	if err != nil {
		return err
	}

	stores, err := buildStores()
	if err != nil {
		return err
	}
	// Provider registry with the stored>env credential fallback: a provider with
	// no stored key falls back to its environment variable (ANTHROPIC_API_KEY,
	// OPENAI_API_KEY, …), so a developer with keys in their shell gets those
	// providers enabled out of the box. Read once — the environment is static for
	// the process. Everything downstream (resolver, providers.list, test) goes
	// through this wrapped registry, so they share one stored>env truth.
	providers := providersvc.WithEnvKeys(stores.Provider, llm.EnvKeys())
	// Seed the registry with the configured provider's credentials (if not
	// already enabled), so the default provider works out of the box. Seeding
	// through the wrapped registry means an env-sourced default isn't redundantly
	// persisted — it stays surfaced as "from env" rather than copied to "stored".
	// Other supported providers stay unconfigured until the user sets their keys.
	if err = seedConfiguredProvider(ctx, providers, cfg); err != nil {
		return err
	}
	// Seed the config-file utility model into its store on first run, so the
	// cheaper maintenance model is honored out of the box; a persisted
	// models.setUtilityRole for the same role wins (runtime edits over config).
	if err = seedUtilityRole(ctx, stores.UtilityRole, cfg); err != nil {
		return err
	}
	// Seed env-sourced MCP servers (LYRA_MCP_SERVERS) into the registry on
	// first run; a persisted workspace.mcp.configure for the same name wins.
	if err = lyraruntime.SeedMCPServers(ctx, stores.MCPServers, runtimeMCPServers(cfg.MCPServers)); err != nil {
		return err
	}

	// User lifecycle hooks: global hooks live at ~/.lyra/hooks.json; a project's
	// at <repo>/.lyra/hooks.json but run only once the project is trusted (a
	// cloned repo's hooks must not auto-execute). Broken hooks are recorded on
	// the turn span. userHome "" (rare) → global hooks just won't be found.
	userHome, _ := os.UserHomeDir()
	hookResolver := adapterhooks.NewResolver(userHome,
		func(hctx context.Context, projectRoot string) bool {
			ok, _ := stores.Trust.IsTrusted(hctx, projectRoot)
			return ok
		},
		func(hctx context.Context, source string, herr error) {
			trace.SpanFromContext(hctx).RecordError(fmt.Errorf("hook %s: %w", source, herr))
		},
	)

	rt, err := lyraruntime.New(ctx, lyraruntime.Config{
		// Engine construction config passes through verbatim (SessionStore
		// is the runtime's to fill — see runtime.Config.Engine).
		Engine: kernel.Config{
			ChatClient: client,
			// Catalog-driven cost: price each round by its served model across
			// every provider, so turns on any provider+model report CostUSD.
			Pricing: lyraruntime.CatalogPricing(),
			// User-scope Agent Skills live under the storage home; per-session
			// project skills (<cwd>/.lyra/skills) layer on top of these.
			SkillsGlobalDir: filepath.Join(stores.Home, "skills"),
			HistoryStore:    stores.ChatHistory,
			Knowledge:       stores.Memory,
			// ProcessStore auto-snapshots every agent process so a parked
			// turn survives a restart (cross-restart HITL resume);
			// ParkStore persists interrupted tool rounds. Both sqlite-backed.
			ProcessStore: stores.Process,
			ParkStore:    stores.Park,
		},
		// Cheaper utility model for compaction / extraction / titling — the
		// runtime resolves it per call from this persisted role (seeded from
		// config.UtilityModel above), falling back to the main client when unset.
		UtilityRoleStore: stores.UtilityRole,
		// Tool-environment inputs — the runtime assembles the tool environment
		// (toolset.Build) from these and injects it into the engine core.
		Online:       lyraruntime.OnlineConfig(cfg.Online),
		MCPRegistry:  stores.MCPServers,
		A2AAgents:    runtimeA2AAgents(cfg.A2AAgents),
		LSPServers:   runtimeLSPServers(cfg.LSPServers), // nil means the built-in LSP server table
		SessionStore: stores.Session,
		// InterruptStore persists the open-interrupt registry that
		// runs.resume looks up — the other half of cross-restart resume.
		InterruptStore:   stores.Interrupt,
		TranscriptStore:  stores.Transcript,
		ProviderRegistry: providers,
		TodoStore:        stores.Todos,
		// Default provider+model a turn runs against when it picks no model.
		Provider: cfg.Provider,
		Model:    cfg.Model,
		// User lifecycle hooks (global + trusted-project), resolved per turn cwd.
		HooksResolver:  hookResolver,
		HookTrustStore: stores.Trust,
		// User-scope prompt recipes under the storage home; per-session project
		// recipes (<cwd>/.lyra/recipes) layer on top of these.
		RecipesGlobalDir: filepath.Join(stores.Home, "recipes"),
		// Scheduled runs (schedules.*) the scheduler worker fires while serving.
		ScheduleRegistry: stores.Schedules,
		// @codebase semantic index: the embedding-model role + the persisted
		// vector index (codebase_search tool + codebase.* RPC).
		EmbeddingRoleStore: stores.EmbeddingRole,
		CodebaseStore:      stores.Codebase,
		Transactor:         lyraruntime.Transactor(stores.Tx),
		// Default approval stance: Balanced — auto-allow file writes /
		// network (the agent's normal work; the user sees the diffs), prompt
		// only on shell exec, the genuinely dangerous class. Must be
		// set explicitly: approval.Mode's zero value is ModeSafe (prompts on
		// EVERY write + exec), which floods a coding session with approvals.
		// Operators flip the mode at runtime; safe/readonly/yolo are opt-in.
		ApprovalMode: approval.ModeBalanced,
		// Persistent fine-grained approval rules (AUX_API §6). nil-default in the
		// runtime treats a missing store as "no remembered rules"; production
		// always wires the sqlite-backed one.
		ApprovalRuleStore: stores.ApprovalRules,
	})
	if err != nil {
		return err
	}
	a.rt = rt
	a.cfg = cfg
	return nil
}
