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

func resolveProviderConfig(settings config.Settings) (config.Settings, error) {
	provider := llm.Provider(settings.Provider)
	if !provider.IsSupported() {
		return config.Settings{}, fmt.Errorf("config: unknown provider %q (see providers.list for the supported set)", settings.Provider)
	}
	if settings.Model == "" {
		settings.Model = provider.DefaultModel()
	}
	apiKeyEnvironmentVariable := provider.APIKeyEnv()
	if apiKey := os.Getenv(apiKeyEnvironmentVariable); apiKey != "" {
		settings.APIKey = apiKey
	}
	if settings.APIKey == "" {
		return config.Settings{}, errors.New("config: apiKey is empty — set it in config/config.yaml or " + apiKeyEnvironmentVariable)
	}
	return settings, nil
}

// DefaultClient builds the provider/model client used when a Run does not
// choose a per-run model.
func DefaultClient(settings config.Settings) (*chatclient.Client, error) {
	return llm.BuildClient(llm.ClientSpec{
		Provider: llm.Provider(settings.Provider),
		Model:    settings.Model,
		APIKey:   settings.APIKey,
		BaseURL:  settings.BaseURL,
	})
}

// ProviderRegistry wraps the durable provider registry with env-key fallback.
func ProviderRegistry(registry models.ProviderRegistry) models.ProviderRegistry {
	return providerregistry.WithEnvironmentKeys(registry, llm.EnvKeys())
}

// MCPServers projects config-file MCP entries into the runtime registry model.
// It rejects an unknown transport instead of preserving an invalid string for a
// later dial attempt; configuration is an input boundary, not a best-effort
// transport pass-through.
func MCPServers(configuredServers []config.MCPServer) ([]mcpserversvc.Server, error) {
	if len(configuredServers) == 0 {
		return nil, nil
	}
	servers := make([]mcpserversvc.Server, len(configuredServers))
	for index, server := range configuredServers {
		transport, err := parseMCPTransport(server.Transport)
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
		servers[index] = candidate
	}
	return servers, nil
}

func parseMCPTransport(transport string) (mcpserversvc.Transport, error) {
	switch transport {
	case config.MCPTransportStreamableHTTP:
		return mcpserversvc.TransportStreamableHTTP, nil
	case config.MCPTransportStdio:
		return mcpserversvc.TransportStdio, nil
	default:
		return "", fmt.Errorf("unknown transport %q", transport)
	}
}
