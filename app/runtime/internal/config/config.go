// Package config loads Lyra's runtime configuration via viper.
//
// Sources, later overrides earlier:
//
//  1. Built-in defaults
//  2. config/config.yaml (or $HOME/.lyra/config.yaml) — viper
//  3. Environment variables (LYRA_*)
//
// The yaml file is where the API key lives in dev; it is gitignored.
// Copy config/config.example.yaml → config/config.yaml and fill it in.
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

// Load resolves configuration from yaml + env + defaults. A missing config
// file is fine (defaults + env only). Provider catalog validation, default
// model selection, and provider-specific API-key env fallback live in the
// composition root because they depend on the LLM adapter catalog, not on
// config-source parsing.
func Load() (Config, error) {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath("config")      // ./config/config.yaml (run from the lyra dir)
	v.AddConfigPath("$HOME/.lyra") // ~/.lyra/config.yaml

	// No default provider — it must be set explicitly in config/config.yaml
	// or via LYRA_PROVIDER. (No vendor is privileged as the implicit default.)
	v.SetDefault("server.listen", "127.0.0.1:17171")
	v.SetDefault("server.noLocalToken", false)

	// LYRA_* env override yaml (e.g. LYRA_PROVIDER, LYRA_SERVER_LISTEN).
	v.SetEnvPrefix("LYRA")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		if _, ok := errors.AsType[viper.ConfigFileNotFoundError](err); !ok {
			return Config{}, fmt.Errorf("config: read config file: %w", err)
		}
		// No config file — defaults + env only.
	}

	provider := v.GetString("provider")
	if provider == "" {
		return Config{}, errors.New("config: provider is required — set `provider:` in config/config.yaml or LYRA_PROVIDER (see providers.list for the supported set)")
	}

	model := v.GetString("model")

	// API key: yaml `apiKey`, optionally overridden later by the composition
	// root with the provider's native env var (e.g. ANTHROPIC_API_KEY).
	apiKey := v.GetString("apiKey")

	servers, err := parseMCPServers(os.Getenv("LYRA_MCP_SERVERS"))
	if err != nil {
		return Config{}, fmt.Errorf("config: LYRA_MCP_SERVERS: %w", err)
	}

	a2aAgents, err := parseA2AAgents(os.Getenv("LYRA_A2A_AGENTS"))
	if err != nil {
		return Config{}, fmt.Errorf("config: LYRA_A2A_AGENTS: %w", err)
	}

	lspServers, err := loadLSPServers(v)
	if err != nil {
		return Config{}, err
	}

	return Config{
		Provider:     provider,
		Model:        model,
		APIKey:       apiKey,
		BaseURL:      v.GetString("baseURL"),
		UtilityModel: v.GetString("utilityModel"),
		Online:       loadOnline(v),
		MCPServers:   servers,
		A2AAgents:    a2aAgents,
		LSPServers:   lspServers,
		Server: ServerConfig{
			Listen:         v.GetString("server.listen"),
			NoLocalToken:   v.GetBool("server.noLocalToken"),
			LocalTokenPath: v.GetString("server.localTokenPath"),
			CORSOrigins:    v.GetStringSlice("server.corsOrigins"),
		},
	}, nil
}
