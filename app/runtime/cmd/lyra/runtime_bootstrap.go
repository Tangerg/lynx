package main

import (
	"context"
	"errors"

	agentruntime "github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/persistence"
	"github.com/Tangerg/lynx/app/runtime/internal/bootstrap"
	"github.com/Tangerg/lynx/app/runtime/internal/config"
)

// bootstrapRuntime builds the composition Host (the application Stack + its
// process-level close order) for the server process.
func bootstrapRuntime(ctx context.Context) (_ bootstrap.Host, _ config.Config, err error) {
	cfg, err := bootstrap.LoadConfig()
	if err != nil {
		return bootstrap.Host{}, config.Config{}, err
	}
	client, err := bootstrap.DefaultClient(cfg)
	if err != nil {
		return bootstrap.Host{}, config.Config{}, err
	}

	stores, err := persistence.Open()
	if err != nil {
		return bootstrap.Host{}, config.Config{}, err
	}
	owned := true
	defer func() {
		if owned {
			err = errors.Join(err, stores.Close())
		}
	}()
	// Reconcile the durable Run-admission table (§8.2) at boot, before any run is
	// admitted: a crash leaves running rows whose live process is gone, which
	// would otherwise block their session forever. Parked (interrupted) runs are
	// preserved for resume. A sweep failure means the DB is unusable, so fail
	// startup rather than admit runs against an inconsistent admission table.
	if _, err := stores.Runs.ReconcileOrphans(ctx, agentruntime.ValidateResumableSnapshot); err != nil {
		return bootstrap.Host{}, config.Config{}, err
	}
	// Provider registry with the stored>env credential fallback: a provider with
	// no stored key falls back to its environment variable (ANTHROPIC_API_KEY,
	// OPENAI_API_KEY, …), so a developer with keys in their shell gets those
	// providers enabled out of the box. Read once — the environment is static for
	// the process. Everything downstream (resolver, providers.list, test) goes
	// through this wrapped registry, so they share one stored>env truth.
	providers := bootstrap.ProviderRegistry(stores.Provider)
	// Seed the registry with the configured provider's credentials (if not
	// already enabled), so the default provider works out of the box. Seeding
	// through the wrapped registry means an env-sourced default isn't redundantly
	// persisted — it stays surfaced as "from env" rather than copied to "stored".
	// Other supported providers stay unconfigured until the user sets their keys.
	if err = bootstrap.SeedConfiguredProvider(ctx, providers, cfg); err != nil {
		return bootstrap.Host{}, config.Config{}, err
	}
	// Seed the config-file utility model into its store on first run, so the
	// cheaper maintenance model is honored out of the box; a persisted
	// models.setUtilityRole for the same role wins (runtime edits over config).
	if err = bootstrap.SeedUtilityRole(ctx, stores.UtilityRole, cfg); err != nil {
		return bootstrap.Host{}, config.Config{}, err
	}
	// Seed env-sourced MCP servers (LYRA_MCP_SERVERS) into the registry on
	// first run; a persisted workspace.mcp.configure for the same name wins.
	if err = bootstrap.SeedMCPServers(ctx, stores.MCPServers, bootstrap.MCPServers(cfg.MCPServers)); err != nil {
		return bootstrap.Host{}, config.Config{}, err
	}

	hookResolver := bootstrap.NewHookResolver(stores.Trust)

	host, err := bootstrap.Assemble(ctx, bootstrap.RuntimeConfig(cfg, stores, client, providers, hookResolver))
	if err != nil {
		return bootstrap.Host{}, config.Config{}, err
	}
	owned = false // successful Runtime construction takes ownership of stores
	return host, cfg, nil
}
