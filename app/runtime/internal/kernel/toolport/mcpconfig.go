package toolport

import (
	"maps"
	"slices"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

// ConfigsForEnabledServers returns the live-connection descriptors projected
// from one registry snapshot (enabled servers only).
func ConfigsForEnabledServers(servers []mcpserver.Server) []MCPServerConfig {
	var out []MCPServerConfig
	for _, s := range servers {
		if s.Enabled {
			out = append(out, ConfigFromServer(s))
		}
	}
	return out
}

// ConfigFromServer maps a persisted registry entry to the live MCP port
// descriptor. Tool-level gating (DisabledTools / AutoApproveTools) is applied at
// toolset build / approval, not at connection setup, so it has no place here.
// Env is flattened from the registry's KEY→value map to the "KEY=value" slice
// the stdio adapter consumes.
func ConfigFromServer(s mcpserver.Server) MCPServerConfig {
	cfg := MCPServerConfig{Name: s.Name, Timeout: s.Timeout}
	switch s.Transport {
	case mcpserver.TransportStreamableHTTP:
		cfg.Transport = MCPTransportHTTP
		cfg.Endpoint = s.URL
		cfg.Authorization = s.Authorization
		cfg.Headers = maps.Clone(s.Headers)
	case mcpserver.TransportStdio:
		cfg.Transport = MCPTransportStdio
		cfg.Command = s.Command
		cfg.Args = slices.Clone(s.Args)
		cfg.Env = envMapToSlice(s.SafeEnv())
		cfg.Dir = s.Dir
	}
	return cfg
}

// envMapToSlice flattens a KEY→value map to the "KEY=value" slice exec wants,
// sorted by key so the dialed env is deterministic (stable across restarts and
// in tests). nil/empty yields nil.
func envMapToSlice(m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	slices.Sort(out)
	return out
}
