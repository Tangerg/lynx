package main

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/persistence"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/startup"
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
	cfg, err := startup.LoadConfig()
	if err != nil {
		return err
	}
	client, err := startup.DefaultClient(cfg)
	if err != nil {
		return err
	}

	stores, err := persistence.Open()
	if err != nil {
		return err
	}
	// Provider registry with the stored>env credential fallback: a provider with
	// no stored key falls back to its environment variable (ANTHROPIC_API_KEY,
	// OPENAI_API_KEY, …), so a developer with keys in their shell gets those
	// providers enabled out of the box. Read once — the environment is static for
	// the process. Everything downstream (resolver, providers.list, test) goes
	// through this wrapped registry, so they share one stored>env truth.
	providers := startup.ProviderRegistry(stores.Provider)
	// Seed the registry with the configured provider's credentials (if not
	// already enabled), so the default provider works out of the box. Seeding
	// through the wrapped registry means an env-sourced default isn't redundantly
	// persisted — it stays surfaced as "from env" rather than copied to "stored".
	// Other supported providers stay unconfigured until the user sets their keys.
	if err = startup.SeedConfiguredProvider(ctx, providers, cfg); err != nil {
		return err
	}
	// Seed the config-file utility model into its store on first run, so the
	// cheaper maintenance model is honored out of the box; a persisted
	// models.setUtilityRole for the same role wins (runtime edits over config).
	if err = startup.SeedUtilityRole(ctx, stores.UtilityRole, cfg); err != nil {
		return err
	}
	// Seed env-sourced MCP servers (LYRA_MCP_SERVERS) into the registry on
	// first run; a persisted workspace.mcp.configure for the same name wins.
	if err = lyraruntime.SeedMCPServers(ctx, stores.MCPServers, startup.MCPServers(cfg.MCPServers)); err != nil {
		return err
	}

	hookResolver := startup.HookResolver(stores.Trust)

	rt, err := lyraruntime.New(ctx, startup.RuntimeConfig(cfg, stores, client, providers, hookResolver))
	if err != nil {
		return err
	}
	a.rt = rt
	a.cfg = cfg
	return nil
}
