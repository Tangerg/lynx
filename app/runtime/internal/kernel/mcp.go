package kernel

import (
	"context"
)

// The MCP connection lifecycle lives outside the engine core. It reaches the
// engine as an [MCPControl] port, so the core exposes workspace.mcp.* without
// importing the concrete MCP adapter.

// MCPServerStatuses returns one entry per configured MCP server (connected and
// failed alike), in dial order.
func (e *Engine) MCPServerStatuses() []MCPServerStatus {
	if e.mcp == nil {
		return nil
	}
	return e.mcp.Statuses()
}

// MCPTools lists the tools advertised by the connected MCP servers, scoped to
// server when non-empty (empty = every connected server).
func (e *Engine) MCPTools(ctx context.Context, server string) ([]MCPToolInfo, error) {
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

// AuthorizeMCPServer runs the interactive OAuth sign-in for an HTTP server and
// hot-swaps the refreshed tool set on success. Returns [ErrUnknownMCPServer]
// for an unconfigured name.
func (e *Engine) AuthorizeMCPServer(ctx context.Context, name string) error {
	if e.mcp == nil {
		return ErrUnknownMCPServer
	}
	return e.mcp.Authorize(ctx, name)
}

// ProbeMCPServer tests a candidate config (workspace.mcp.test). It routes
// through the live connections so an active OAuth sign-in for the same-named
// server is reused — otherwise an authorized server would 401 on the anonymous
// probe and read as "unauthorized".
func (e *Engine) ProbeMCPServer(ctx context.Context, cfg MCPServerConfig) error {
	if e.mcp == nil {
		return ErrUnknownMCPServer
	}
	return e.mcp.Probe(ctx, cfg)
}

// ConfigureMCPServer adds or re-dials a server in the live connection set and
// hot-swaps the refreshed model-facing tool set. A dial failure is returned but
// the server is still tracked (recorded "failed", reconnectable). Nil MCP
// (no servers wired) is a wiring bug for a configure, so it errors.
func (e *Engine) ConfigureMCPServer(ctx context.Context, cfg MCPServerConfig) error {
	if e.mcp == nil {
		return ErrUnknownMCPServer
	}
	return e.mcp.Configure(ctx, cfg)
}

// RemoveMCPServer drops a server from the live connection set (closing its
// session) and hot-swaps the refreshed tool set. Unknown name is a no-op.
func (e *Engine) RemoveMCPServer(ctx context.Context, name string) {
	if e.mcp == nil {
		return
	}
	e.mcp.Remove(ctx, name)
}
