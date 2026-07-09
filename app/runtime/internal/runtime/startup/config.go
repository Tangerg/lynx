// Package startup adapts process config and environment state into runtime
// construction inputs.
package startup

import (
	"errors"
	"fmt"
	"os"

	"github.com/Tangerg/lynx/app/runtime/internal/config"
	mcpserversvc "github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	providersvc "github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/llm"
	lyraruntime "github.com/Tangerg/lynx/app/runtime/internal/runtime"
	"github.com/Tangerg/lynx/core/model/chat"
)

// LoadConfig loads the app config and resolves provider defaults plus env-key
// overrides used by the runtime process.
func LoadConfig() (config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return config.Config{}, err
	}
	return resolveProviderConfig(cfg)
}

func resolveProviderConfig(cfg config.Config) (config.Config, error) {
	provider := llm.Provider(cfg.Provider)
	if !llm.IsSupported(provider) {
		return config.Config{}, fmt.Errorf("config: unknown provider %q (see providers.list for the supported set)", cfg.Provider)
	}
	if cfg.Model == "" {
		cfg.Model = llm.DefaultModel(provider)
	}
	apiKeyEnv := llm.APIKeyEnv(provider)
	if envKey := os.Getenv(apiKeyEnv); envKey != "" {
		cfg.APIKey = envKey
	}
	if cfg.APIKey == "" {
		return config.Config{}, errors.New("config: apiKey is empty — set it in config/config.yaml or " + apiKeyEnv)
	}
	return cfg, nil
}

// DefaultClient builds the provider/model client used when a turn does not
// choose a per-run model.
func DefaultClient(cfg config.Config) (*chat.Client, error) {
	return llm.BuildClient(llm.ClientSpec{
		Provider: llm.Provider(cfg.Provider),
		Model:    cfg.Model,
		APIKey:   cfg.APIKey,
		BaseURL:  cfg.BaseURL,
	})
}

// ProviderRegistry wraps the durable provider registry with env-key fallback.
func ProviderRegistry(reg providersvc.Registry) providersvc.Registry {
	return providersvc.WithEnvKeys(reg, llm.EnvKeys())
}

// MCPServers projects config-file MCP entries into the runtime registry model.
func MCPServers(in []config.MCPServerConfig) []mcpserversvc.Server {
	if len(in) == 0 {
		return nil
	}
	out := make([]mcpserversvc.Server, len(in))
	for i, server := range in {
		out[i] = mcpserversvc.Server{
			Name:          server.Name,
			Transport:     runtimeMCPTransport(server.Transport),
			Enabled:       true,
			URL:           server.Endpoint,
			Authorization: server.Authorization,
			Command:       server.Command,
			Args:          append([]string(nil), server.Args...),
		}
	}
	return out
}

func runtimeMCPTransport(transport string) string {
	switch transport {
	case config.MCPTransportStreamableHTTP:
		return mcpserversvc.TransportStreamableHTTP
	case config.MCPTransportStdio:
		return mcpserversvc.TransportStdio
	default:
		return transport
	}
}

func runtimeA2AAgents(in []config.A2AAgentConfig) []lyraruntime.A2AAgentConfig {
	if len(in) == 0 {
		return nil
	}
	out := make([]lyraruntime.A2AAgentConfig, len(in))
	for i, agent := range in {
		out[i] = lyraruntime.A2AAgentConfig{
			Name:    agent.Name,
			CardURL: agent.CardURL,
		}
	}
	return out
}

func runtimeLSPServers(in []config.LSPServerConfig) []lyraruntime.LSPServerConfig {
	if len(in) == 0 {
		return nil
	}
	out := make([]lyraruntime.LSPServerConfig, len(in))
	for i, server := range in {
		out[i] = lyraruntime.LSPServerConfig{
			Name:        server.Name,
			Command:     server.Command,
			Args:        server.Args,
			LanguageID:  server.LanguageID,
			Extensions:  server.Extensions,
			RootMarkers: server.RootMarkers,
		}
	}
	return out
}
