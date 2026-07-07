package runtime

import (
	"context"
	"slices"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
)

// enabledConfigs reads the registry and returns the live-connection port
// descriptors for the enabled servers — the boot-time MCP set handed to
// toolset.Build.
func enabledConfigs(ctx context.Context, svc mcpserver.Registry) ([]kernel.MCPServerConfig, error) {
	servers, err := svc.List(ctx)
	if err != nil {
		return nil, err
	}
	var out []kernel.MCPServerConfig
	for _, s := range servers {
		if s.Enabled {
			out = append(out, configFromServer(s))
		}
	}
	return out, nil
}

// configFromServer maps a registry entry to the kernel's live MCP port
// descriptor. Tool-level gating (DisabledTools / AutoApproveTools) is applied
// at toolset build / approval, not at connection setup, so it has no place
// here. Env is flattened from the registry's KEY→value map to the "KEY=value"
// slice the stdio adapter consumes.
func configFromServer(s mcpserver.Server) kernel.MCPServerConfig {
	cfg := kernel.MCPServerConfig{Name: s.Name, Timeout: s.Timeout}
	switch s.Transport {
	case mcpserver.TransportStreamableHTTP:
		cfg.Transport = kernel.MCPTransportHTTP
		cfg.Endpoint = s.URL
		cfg.Authorization = s.Authorization
		cfg.Headers = s.Headers
	case mcpserver.TransportStdio:
		cfg.Transport = kernel.MCPTransportStdio
		cfg.Command = s.Command
		cfg.Args = s.Args
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
