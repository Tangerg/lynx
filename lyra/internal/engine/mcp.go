package engine

import (
	"context"

	"github.com/Tangerg/lynx/lyra/internal/infra/mcp"
)

// The MCP connection lifecycle (dial / sessions / reconnect / tool hot-swap)
// lives in infra/mcp. The engine holds one [mcp.Connections] and exposes a thin
// facade for the runtime SPI / workspace.mcp.* wire surface; the wire
// projections are re-exported so those callers name one type.

// McpToolInfo is one tool advertised by a connected MCP server (workspace.mcp.
// listTools).
type McpToolInfo = mcp.ToolInfo

// McpServerStatus is the per-server connection state (workspace.mcp.listServers).
type McpServerStatus = mcp.ServerStatus

// ErrUnknownMCPServer is returned by [Engine.ReconnectMCPServer] for an
// unconfigured name; workspace.mcp.reconnect maps it to invalid_params.
var ErrUnknownMCPServer = mcp.ErrUnknownServer

// MCPServerStatuses returns one entry per configured MCP server (connected and
// failed alike), in dial order.
func (e *Engine) MCPServerStatuses() []McpServerStatus { return e.mcp.Statuses() }

// MCPTools lists the tools advertised by the connected MCP servers, scoped to
// server when non-empty (empty = every connected server).
func (e *Engine) MCPTools(ctx context.Context, server string) ([]McpToolInfo, error) {
	return e.mcp.Tools(ctx, server)
}

// ReconnectMCPServer re-dials a configured server and hot-swaps the refreshed
// model-facing MCP tool set. Returns [ErrUnknownMCPServer] for an unconfigured
// name.
func (e *Engine) ReconnectMCPServer(ctx context.Context, name string) error {
	return e.mcp.Reconnect(ctx, name)
}
