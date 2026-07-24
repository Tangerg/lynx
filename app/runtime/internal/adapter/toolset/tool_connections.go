package toolset

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/infra/a2a"
	"github.com/Tangerg/lynx/tools"
)

type a2aConnections struct {
	a2a      *a2a.Connections
	a2aTools []tools.Tool
}

// dialA2AConnections establishes the remote delegation family owned by the
// toolset. MCP uses a different lifecycle and is owned by mcpconnection.
func dialA2AConnections(ctx context.Context, config BuildConfig) (a2aConnections, error) {
	a2aConns, a2aTools, err := a2a.Dial(ctx, infraA2AClientConfigs(config.A2AAgents))
	if err != nil {
		return a2aConnections{}, err
	}
	return a2aConnections{a2a: a2aConns, a2aTools: a2aTools}, nil
}
