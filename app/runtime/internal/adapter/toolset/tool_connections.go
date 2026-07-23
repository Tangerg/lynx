package toolset

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/infra/a2a"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/mcp"
	"github.com/Tangerg/lynx/tools"
)

type liveToolConnections struct {
	mcp      *mcp.Connections
	mcpTools []tools.Tool
	a2a      *a2a.Connections
	a2aTools []tools.Tool
}

// dialToolConnections establishes the two remote tool families as one build
// step. On a partial failure Build's deferred cleanup receives the successful
// connection and closes it along with the local capability adapters.
func dialToolConnections(ctx context.Context, config BuildConfig) (liveToolConnections, error) {
	mcpConns, mcpTools, err := mcp.Dial(ctx, infraMCPServerConfigs(config.MCPServers))
	if err != nil {
		return liveToolConnections{}, err
	}
	a2aConns, a2aTools, err := a2a.Dial(ctx, infraA2AClientConfigs(config.A2AAgents))
	if err != nil {
		return liveToolConnections{mcp: mcpConns}, err
	}
	return liveToolConnections{
		mcp: mcpConns, mcpTools: mcpTools,
		a2a: a2aConns, a2aTools: a2aTools,
	}, nil
}
