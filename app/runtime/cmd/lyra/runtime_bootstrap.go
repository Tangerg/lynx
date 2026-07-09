package main

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/otel/trace"

	adapterhooks "github.com/Tangerg/lynx/app/runtime/internal/adapter/hooks"
	providersvc "github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/llm"
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

	rt, err := lyraruntime.New(ctx, buildRuntimeConfig(cfg, stores, client, providers, hookResolver))
	if err != nil {
		return err
	}
	a.rt = rt
	a.cfg = cfg
	return nil
}
