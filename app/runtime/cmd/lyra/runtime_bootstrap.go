package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/persistence"
	"github.com/Tangerg/lynx/app/runtime/internal/bootstrap"
	"github.com/Tangerg/lynx/app/runtime/internal/config"
)

// runtimePaths is the executable's single snapshot of host paths. User Home,
// default workspace, durable data, and launch directory are distinct facts even
// when defaults make some of them descendants or aliases; inner runtime rings
// consume these values and never rediscover them.
type runtimePaths struct {
	userHome             string
	defaultWorkspacePath string
	dataDirectory        string
	launchDirectory      string
}

func resolveRuntimePaths() (runtimePaths, error) {
	userHome, err := os.UserHomeDir()
	if err != nil {
		return runtimePaths{}, fmt.Errorf("runtime: locate user home: %w", err)
	}
	if userHome == "" {
		return runtimePaths{}, errors.New("runtime: user home is unavailable")
	}
	if !filepath.IsAbs(userHome) {
		return runtimePaths{}, errors.New("runtime: user home must be absolute")
	}
	userHome = filepath.Clean(userHome)
	launchDirectory, err := os.Getwd()
	if err != nil {
		return runtimePaths{}, fmt.Errorf("runtime: locate launch directory: %w", err)
	}
	if !filepath.IsAbs(launchDirectory) {
		return runtimePaths{}, errors.New("runtime: launch directory must be absolute")
	}
	dataDirectory := os.Getenv("LYRA_HOME")
	if dataDirectory == "" {
		dataDirectory = filepath.Join(userHome, ".lyra")
	} else if !filepath.IsAbs(dataDirectory) {
		return runtimePaths{}, errors.New("runtime: LYRA_HOME must be an absolute path")
	}
	return runtimePaths{
		userHome:             userHome,
		defaultWorkspacePath: userHome,
		dataDirectory:        filepath.Clean(dataDirectory),
		launchDirectory:      filepath.Clean(launchDirectory),
	}, nil
}

// bootstrapRuntime builds the composition Host (the application Stack + its
// process-level close order) for the server process.
func bootstrapRuntime(ctx context.Context) (_ *bootstrap.Host, _ config.Settings, _ runtimePaths, err error) {
	paths, err := resolveRuntimePaths()
	if err != nil {
		return nil, config.Settings{}, runtimePaths{}, err
	}
	host, cfg, err := bootstrapRuntimeWithBuildID(ctx, paths, bootstrap.ExecutableBuildID)
	return host, cfg, paths, err
}

func bootstrapRuntimeWithBuildID(ctx context.Context, paths runtimePaths, buildIdentity func() (string, error)) (_ *bootstrap.Host, _ config.Settings, err error) {
	buildID, err := buildIdentity()
	if err != nil {
		return nil, config.Settings{}, err
	}
	cfg, err := bootstrap.LoadConfig([]string{
		filepath.Join(paths.launchDirectory, "config"),
		paths.dataDirectory,
	})
	if err != nil {
		return nil, config.Settings{}, err
	}
	client, err := bootstrap.DefaultClient(cfg)
	if err != nil {
		return nil, config.Settings{}, err
	}

	stores, err := persistence.Open(ctx, persistence.Config{
		DataDirectory:        paths.dataDirectory,
		DefaultWorkspacePath: paths.defaultWorkspacePath,
	})
	if err != nil {
		return nil, config.Settings{}, err
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
	// through the wrapped registry means an env-sourced default key isn't
	// redundantly persisted — it stays surfaced as "from env" rather than copied
	// to "stored" — while a required custom endpoint is still persisted by itself.
	// Other supported providers stay unconfigured until the user sets their keys.
	if err = bootstrap.SeedConfiguredProvider(ctx, providers, cfg); err != nil {
		return nil, config.Settings{}, err
	}
	// Seed the config-file utility model into its store on first run, so the
	// cheaper maintenance model is honored out of the box; a persisted
	// models.setUtilityRole for the same role wins (runtime edits over config).
	if err = bootstrap.SeedUtilityRole(ctx, stores.UtilityRole, cfg); err != nil {
		return nil, config.Settings{}, err
	}
	// Seed env-sourced MCP servers (LYRA_MCP_SERVERS) into the registry on
	// first run; a persisted mcp.servers resource with the same name wins.
	mcpServers, err := bootstrap.MCPServers(cfg.MCPServers)
	if err != nil {
		return nil, config.Settings{}, err
	}
	if err = bootstrap.SeedMCPServers(ctx, stores.MCPServers, mcpServers); err != nil {
		return nil, config.Settings{}, err
	}

	hookResolver := bootstrap.NewHookResolver(paths.userHome, stores.Trust)

	runtimeCfg := bootstrap.ComposeConfig(cfg, stores, client, providers, hookResolver, buildID)
	runtimeCfg.UserHome = paths.userHome
	runtimeCfg.DefaultWorkspacePath = paths.defaultWorkspacePath
	assembly := bootstrap.NewAssembly(runtimeCfg)
	owned = false
	defer func() {
		err = errors.Join(err, bootstrap.CloseAssembly(assembly))
	}()
	host, err := bootstrap.BuildAssembly(ctx, assembly)
	if err != nil {
		return nil, config.Settings{}, err
	}
	return host, cfg, nil
}
