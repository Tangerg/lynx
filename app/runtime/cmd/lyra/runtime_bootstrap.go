package main

import (
	"context"
	"errors"
	"os"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/persistence"
	"github.com/Tangerg/lynx/app/runtime/internal/bootstrap"
	"github.com/Tangerg/lynx/app/runtime/internal/config"
)

// bootstrapRuntime builds the composition Host (the application Stack + its
// process-level close order) for the server process.
func bootstrapRuntime(ctx context.Context) (_ bootstrap.Host, _ config.Config, err error) {
	return bootstrapRuntimeWithBuildID(ctx, bootstrap.ExecutableBuildID)
}

func bootstrapRuntimeWithBuildID(ctx context.Context, buildIdentity func() (string, error)) (_ bootstrap.Host, _ config.Config, err error) {
	buildID, err := buildIdentity()
	if err != nil {
		return bootstrap.Host{}, config.Config{}, err
	}
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
	// first run; a persisted mcp.configs.configure for the same name wins.
	if err = bootstrap.SeedMCPServers(ctx, stores.MCPServers, bootstrap.MCPServers(cfg.MCPServers)); err != nil {
		return bootstrap.Host{}, config.Config{}, err
	}

	hookResolver := bootstrap.NewHookResolver(stores.Trust)

	runtimeCfg := bootstrap.RuntimeConfig(cfg, stores, client, providers, hookResolver, buildID)
	if cwd, cwdErr := os.UserHomeDir(); cwdErr == nil {
		runtimeCfg.DefaultCwd = cwd
	}
	host, err := bootstrap.Assemble(ctx, runtimeCfg)
	if err != nil {
		return bootstrap.Host{}, config.Config{}, err
	}
	owned = false // successful Runtime construction takes ownership of stores
	return host, cfg, nil
}
