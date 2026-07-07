package toolset

import (
	"maps"
	"slices"

	"github.com/Tangerg/lynx/app/runtime/internal/infra/mcp"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/toolport"
)

func infraMCPServerConfigs(in []toolport.MCPServerConfig) []mcp.ServerConfig {
	if len(in) == 0 {
		return nil
	}
	out := make([]mcp.ServerConfig, len(in))
	for i, server := range in {
		out[i] = infraMCPServerConfig(server)
	}
	return out
}

func infraMCPServerConfig(in toolport.MCPServerConfig) mcp.ServerConfig {
	return mcp.ServerConfig{
		Name:          in.Name,
		Transport:     infraMCPTransport(in.Transport),
		Endpoint:      in.Endpoint,
		Command:       in.Command,
		Args:          slices.Clone(in.Args),
		Env:           slices.Clone(in.Env),
		Dir:           in.Dir,
		Authorization: in.Authorization,
		Headers:       maps.Clone(in.Headers),
		Timeout:       in.Timeout,
	}
}

func infraMCPTransport(in toolport.MCPTransport) mcp.Transport {
	switch in {
	case toolport.MCPTransportHTTP:
		return mcp.TransportHTTP
	case toolport.MCPTransportStdio:
		return mcp.TransportStdio
	default:
		return 0
	}
}
