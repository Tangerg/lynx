// Package bootstrap is the composition root: it adapts process config and
// environment into runtime construction inputs, wires the rings, and owns host
// lifecycle.
package bootstrap

import (
	"errors"
	"fmt"
	"os"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/providerregistry"
	"github.com/Tangerg/lynx/app/runtime/internal/application/models"
	"github.com/Tangerg/lynx/app/runtime/internal/config"
	mcpserversvc "github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/llm"
	"github.com/Tangerg/lynx/chatclient"
)

// LoadConfig loads the app config and resolves provider defaults plus env-key
// overrides used by the runtime process.
func LoadConfig(configDirectories []string) (config.Settings, error) {
	cfg, err := config.Load(configDirectories)
	if err != nil {
		return config.Settings{}, err
	}
	return resolveProviderConfig(cfg)
}

func resolveProviderConfig(cfg config.Settings) (config.Settings, error) {
	provider := llm.Provider(cfg.Provider)
	if !provider.IsSupported() {
		return config.Settings{}, fmt.Errorf("config: unknown provider %q (see providers.list for the supported set)", cfg.Provider)
	}
	if cfg.Model == "" {
		cfg.Model = provider.DefaultModel()
	}
	apiKeyEnv := provider.APIKeyEnv()
	if envKey := os.Getenv(apiKeyEnv); envKey != "" {
		cfg.APIKey = envKey
	}
	if cfg.APIKey == "" {
		return config.Settings{}, errors.New("config: apiKey is empty — set it in config/config.yaml or " + apiKeyEnv)
	}
	return cfg, nil
}

// DefaultClient builds the provider/model client used when a Run does not
// choose a per-run model.
func DefaultClient(cfg config.Settings) (*chatclient.Client, error) {
	return llm.BuildClient(llm.ClientSpec{
		Provider: llm.Provider(cfg.Provider),
		Model:    cfg.Model,
		APIKey:   cfg.APIKey,
		BaseURL:  cfg.BaseURL,
	})
}

// ProviderRegistry wraps the durable provider registry with env-key fallback.
func ProviderRegistry(reg models.ProviderRegistry) models.ProviderRegistry {
	return providerregistry.WithEnvironmentKeys(reg, llm.EnvKeys())
}

// MCPServers projects config-file MCP entries into the runtime registry model.
// It rejects an unknown transport instead of preserving an invalid string for a
// later dial attempt; configuration is an input boundary, not a best-effort
// transport pass-through.
func MCPServers(in []config.MCPServer) ([]mcpserversvc.Server, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := make([]mcpserversvc.Server, len(in))
	for i, server := range in {
		transport, err := runtimeMCPTransport(server.Transport)
		if err != nil {
			return nil, fmt.Errorf("config: MCP server %q: %w", server.Name, err)
		}
		candidate := mcpserversvc.Server{
			Name:          server.Name,
			Transport:     transport,
			Enabled:       true,
			URL:           server.Endpoint,
			Authorization: server.Authorization,
			Command:       server.Command,
			Args:          append([]string(nil), server.Args...),
		}
		if err := candidate.Validate(); err != nil {
			return nil, fmt.Errorf("config: MCP server %q: %w", server.Name, err)
		}
		out[i] = candidate
	}
	return out, nil
}

func runtimeMCPTransport(transport string) (mcpserversvc.Transport, error) {
	switch transport {
	case config.MCPTransportStreamableHTTP:
		return mcpserversvc.TransportStreamableHTTP, nil
	case config.MCPTransportStdio:
		return mcpserversvc.TransportStdio, nil
	default:
		return "", fmt.Errorf("unknown transport %q", transport)
	}
}
