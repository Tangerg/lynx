package engine

import (
	"context"

	"github.com/Tangerg/lynx/lyra/internal/engine/toolset"
)

// The MCP connection lifecycle lives in infra/mcp; it's constructed in the
// toolset assembly layer and reaches the engine core as a [toolset.MCPControl]
// port (e.mcp) — so the engine core imports no infra/mcp. The engine exposes a
// thin facade for the runtime SPI / workspace.mcp.* wire surface; the wire
// projections are re-exported so those callers name one type.

// McpToolInfo is one tool advertised by a connected MCP server (workspace.mcp.
// listTools).
type McpToolInfo = toolset.MCPToolInfo

// McpServerStatus is the per-server connection state (workspace.mcp.listServers).
type McpServerStatus = toolset.MCPServerStatus

// ErrUnknownMCPServer is returned by [Engine.ReconnectMCPServer] for an
// unconfigured name; workspace.mcp.reconnect maps it to invalid_params.
var ErrUnknownMCPServer = toolset.ErrUnknownMCPServer

// MCPServerStatuses returns one entry per configured MCP server (connected and
// failed alike), in dial order.
func (e *Engine) MCPServerStatuses() []McpServerStatus {
	if e.mcp == nil {
		return nil
	}
	return e.mcp.Statuses()
}

// MCPTools lists the tools advertised by the connected MCP servers, scoped to
// server when non-empty (empty = every connected server).
func (e *Engine) MCPTools(ctx context.Context, server string) ([]McpToolInfo, error) {
	if e.mcp == nil {
		return nil, nil
	}
	return e.mcp.Tools(ctx, server)
}

// ReconnectMCPServer re-dials a configured server and hot-swaps the refreshed
// model-facing MCP tool set. Returns [ErrUnknownMCPServer] for an unconfigured
// name.
func (e *Engine) ReconnectMCPServer(ctx context.Context, name string) error {
	if e.mcp == nil {
		return ErrUnknownMCPServer
	}
	return e.mcp.Reconnect(ctx, name)
}
