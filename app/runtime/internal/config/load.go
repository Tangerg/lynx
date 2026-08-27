// Package config loads ScopeApp's runtime settings via Viper.
//
// Sources, later overrides earlier:
//
//  1. Built-in defaults
//  2. config.yaml in an executable-supplied absolute search directory
//  3. Environment variables (SCOPEAPP_*)
//
// The yaml file is where the API key lives in dev; it is gitignored.
// Copy config/config.example.yaml → config/config.yaml and fill it in.
package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// Load resolves configuration from yaml + env + defaults. A missing config
// file is fine (defaults + env only). Provider catalog validation, default
// model selection, and provider-specific API-key fallback are deliberately
// outside config-source parsing because they depend on the live provider
// catalog.
func Load(configDirectories []string) (Settings, error) {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	for _, directory := range configDirectories {
		if !filepath.IsAbs(directory) {
			return Settings{}, fmt.Errorf("config: search directory %q must be absolute", directory)
		}
		v.AddConfigPath(filepath.Clean(directory))
	}

	// No default provider — it must be set explicitly in config/config.yaml
	// or via SCOPEAPP_PROVIDER. (No vendor is privileged as the implicit default.)
	v.SetDefault("server.listen", "127.0.0.1:17171")
	v.SetDefault("server.noLocalToken", false)
	// Tool-result eviction is on by default; an explicit non-positive value
	// (e.g. toolResultOffload.threshold: 0) disables it.
	v.SetDefault("toolResultOffload.threshold", DefaultToolResultOffloadThreshold)

	// SCOPEAPP_* env override yaml (e.g. SCOPEAPP_PROVIDER, SCOPEAPP_SERVER_LISTEN).
	v.SetEnvPrefix(environmentPrefix.String())
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		if _, ok := errors.AsType[viper.ConfigFileNotFoundError](err); !ok {
			return Settings{}, fmt.Errorf("config: read config file: %w", err)
		}
		// No config file — defaults + env only.
	}

	provider := v.GetString("provider")
	if provider == "" {
		return Settings{}, errors.New("config: provider is required — set `provider:` in config/config.yaml or SCOPEAPP_PROVIDER (see providers.list for the supported set)")
	}

	model := v.GetString("model")

	// API key from yaml `apiKey` or SCOPEAPP_APIKEY. Provider-native environment
	// fallback is resolved separately against the selected provider.
	apiKey := v.GetString("apiKey")

	servers, err := parseMCPServers(mcpServersEnvironment.Value())
	if err != nil {
		return Settings{}, fmt.Errorf("config: %s: %w", mcpServersEnvironment, err)
	}

	a2aAgents, err := parseA2AAgents(a2aAgentsEnvironment.Value())
	if err != nil {
		return Settings{}, fmt.Errorf("config: %s: %w", a2aAgentsEnvironment, err)
	}
	a2aAgents, err = addA2ARPCOrigins(a2aAgents, a2aOriginsEnvironment.Value())
	if err != nil {
		return Settings{}, fmt.Errorf("config: %s: %w", a2aOriginsEnvironment, err)
	}

	lspServers, err := loadLSPServers(v)
	if err != nil {
		return Settings{}, err
	}

	return Settings{
		Provider:     provider,
		Model:        model,
		APIKey:       apiKey,
		BaseURL:      v.GetString("baseURL"),
		UtilityModel: v.GetString("utilityModel"),
		Online:       loadOnline(v),
		MCPServers:   servers,
		A2AAgents:    a2aAgents,
		LSPServers:   lspServers,

		ToolResultOffloadThreshold: v.GetInt("toolResultOffload.threshold"),

		SandboxShell:         v.GetBool("sandbox.shell"),
		SandboxReadOnlyPaths: v.GetStringSlice("sandbox.readOnlyPaths"),

		Server: Server{
			Listen:         v.GetString("server.listen"),
			NoLocalToken:   v.GetBool("server.noLocalToken"),
			LocalTokenPath: v.GetString("server.localTokenPath"),
			CORSOrigins:    v.GetStringSlice("server.corsOrigins"),
		},
	}, nil
}
