package config

import (
	"fmt"

	"github.com/spf13/viper"
)

// loadLSPServers reads the optional language-server table from yaml
// `lsp.servers`. Absent → nil (the engine falls back to lsp.DefaultServers()).
// mapstructure matches keys case-insensitively, so the yaml keys are
// name/command/args/languageId/extensions/rootMarkers.
func loadLSPServers(v *viper.Viper) ([]LSPServerConfig, error) {
	var servers []LSPServerConfig
	if err := v.UnmarshalKey("lsp.servers", &servers); err != nil {
		return nil, fmt.Errorf("config: lsp.servers: %w", err)
	}
	return servers, nil
}
