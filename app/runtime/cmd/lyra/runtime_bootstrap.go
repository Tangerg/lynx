package main

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/persistence"
	"github.com/Tangerg/lynx/app/runtime/internal/application/schedules"
	"github.com/Tangerg/lynx/app/runtime/internal/bootstrap"
	"github.com/Tangerg/lynx/app/runtime/internal/config"
	lyraruntime "github.com/Tangerg/lynx/app/runtime/internal/runtime"
)

// runtimeStack is the assembled application the delivery layer runs on: the
// (shrinking) Runtime facade plus the extracted application coordinators that
// delivery holds directly. Batch 4 grows this as the facade is dismantled.
type runtimeStack struct {
	rt        *lyraruntime.Runtime
	schedules *schedules.Coordinator
}

// bootstrapRuntime builds the runtime bundle + application coordinators for the
// server process.
func bootstrapRuntime(ctx context.Context) (_ runtimeStack, _ config.Config, err error) {
	cfg, err := bootstrap.LoadConfig()
	if err != nil {
		return runtimeStack{}, config.Config{}, err
	}
	client, err := bootstrap.DefaultClient(cfg)
	if err != nil {
		return runtimeStack{}, config.Config{}, err
	}

	stores, err := persistence.Open()
	if err != nil {
		return runtimeStack{}, config.Config{}, err
	}
	owned := true
	defer func() {
		if owned {
			err = errors.Join(err, stores.Close())
		}
	}()
	// The schedule store is both the CRUD registry and the worker's due-scan
	// store; the coordinator holds it and the composition transfers store
	// ownership to the Runtime below (its Close covers the shared bundle).
	scheduleCoord := schedules.NewCoordinator(stores.Schedules, stores.Schedules)
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
		return runtimeStack{}, config.Config{}, err
	}
	// Seed the config-file utility model into its store on first run, so the
	// cheaper maintenance model is honored out of the box; a persisted
	// models.setUtilityRole for the same role wins (runtime edits over config).
	if err = bootstrap.SeedUtilityRole(ctx, stores.UtilityRole, cfg); err != nil {
		return runtimeStack{}, config.Config{}, err
	}
	// Seed env-sourced MCP servers (LYRA_MCP_SERVERS) into the registry on
	// first run; a persisted workspace.mcp.configure for the same name wins.
	if err = lyraruntime.SeedMCPServers(ctx, stores.MCPServers, bootstrap.MCPServers(cfg.MCPServers)); err != nil {
		return runtimeStack{}, config.Config{}, err
	}

	hookResolver := bootstrap.HookResolver(stores.Trust)

	rt, err := lyraruntime.New(ctx, bootstrap.RuntimeConfig(cfg, stores, client, providers, hookResolver))
	if err != nil {
		return runtimeStack{}, config.Config{}, err
	}
	owned = false // successful Runtime construction takes ownership of stores
	return runtimeStack{rt: rt, schedules: scheduleCoord}, cfg, nil
}
