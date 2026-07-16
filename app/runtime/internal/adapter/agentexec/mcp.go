package agentexec

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

// The MCP connection lifecycle lives outside the engine core. It reaches the
// engine through narrow live-MCP ports, so the core exposes workspace.mcp.*
// without importing the concrete MCP adapter.

// MCPServerStatuses returns one entry per configured MCP server (connected and
// failed alike), in dial order.
func (e *Engine) MCPServerStatuses() []mcpserver.ConnectionStatus {
	if e.mcpStatusReader == nil {
		return nil
	}
	return e.mcpStatusReader.Statuses()
}

// MCPTools lists the tools advertised by the connected MCP servers, scoped to
// server when non-empty (empty = every connected server).
func (e *Engine) MCPTools(ctx context.Context, server string) ([]mcpserver.ToolInfo, error) {
	if e.mcpToolCatalog == nil {
		return nil, nil
	}
	return e.mcpToolCatalog.Tools(ctx, server)
}

// ReconnectMCPServer re-dials a configured server and hot-swaps the refreshed
// model-facing MCP tool set. Returns [mcpserver.ErrUnknownServer] for an unconfigured
// name.
func (e *Engine) ReconnectMCPServer(ctx context.Context, name string) error {
	if e.mcpConnectionCommands == nil {
		return mcpserver.ErrUnknownServer
	}
	return e.mcpConnectionCommands.Reconnect(ctx, name)
}

// AuthorizeMCPServer runs the interactive OAuth sign-in for an HTTP server and
// hot-swaps the refreshed tool set on success. Returns [mcpserver.ErrUnknownServer]
// for an unconfigured name.
func (e *Engine) AuthorizeMCPServer(ctx context.Context, name string) error {
	if e.mcpConnectionCommands == nil {
		return mcpserver.ErrUnknownServer
	}
	return e.mcpConnectionCommands.Authorize(ctx, name)
}

// ProbeMCPServer tests a candidate config (workspace.mcp.test). It routes
// through the live connections so an active OAuth sign-in for the same-named
// server is reused — otherwise an authorized server would 401 on the anonymous
// probe and read as "unauthorized".
func (e *Engine) ProbeMCPServer(ctx context.Context, config mcpserver.LiveConfig) error {
	if e.mcpRegistryCommands == nil {
		return mcpserver.ErrUnknownServer
	}
	return e.mcpRegistryCommands.Probe(ctx, config)
}

// ConfigureMCPServer adds or re-dials a server in the live connection set and
// hot-swaps the refreshed model-facing tool set. A dial failure is returned but
// the server is still tracked (recorded "failed", reconnectable). Nil MCP
// (no servers wired) is a wiring bug for a configure, so it errors.
func (e *Engine) ConfigureMCPServer(ctx context.Context, config mcpserver.LiveConfig) error {
	if e.mcpRegistryCommands == nil {
		return mcpserver.ErrUnknownServer
	}
	return e.mcpRegistryCommands.Configure(ctx, config)
}

// RemoveMCPServer drops a server from the live connection set (closing its
// session) and hot-swaps the refreshed tool set. Unknown name is a no-op.
func (e *Engine) RemoveMCPServer(ctx context.Context, name string) {
	if e.mcpRegistryCommands == nil {
		return
	}
	e.mcpRegistryCommands.Remove(ctx, name)
}
