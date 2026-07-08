package runtime

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
)

// MCP-server registry orchestration: the runtime owns both the persisted
// registry (mcpserver.Registry) and the live connections (via the engine's
// MCP registry command port), so a configure/remove/enable both persists and
// applies to the live tool set in one place. Registry entries are projected to
// dial-level descriptors only at the live-connection boundary.

// ListMCPRegisteredServers returns the persisted MCP-server registry entries,
// distinct from the live connection statuses returned by MCPServerStatuses.
func (r *Runtime) ListMCPRegisteredServers(ctx context.Context) ([]mcpserver.Server, error) {
	return r.mcpRegistry.List(ctx)
}

// MCPServerStatuses returns the per-server connection state of every
// configured MCP server (connected and boot-failed alike) for
// workspace.mcp.listServers. Delegates to the engine, which owns the sessions.
func (r *Runtime) MCPServerStatuses() []kernel.MCPServerStatus {
	return r.engine.MCPServerStatuses()
}

// MCPRegisteredServer returns one persisted MCP-server registry entry.
func (r *Runtime) MCPRegisteredServer(ctx context.Context, name string) (mcpserver.Server, bool, error) {
	return r.mcpRegistry.Get(ctx, name)
}

// ReconnectMCPServer re-dials a configured MCP server and hot-swaps the live
// tool set (workspace.mcp.reconnect). Delegates to the engine, which owns the
// sessions + the shared client.
func (r *Runtime) ReconnectMCPServer(ctx context.Context, name string) error {
	return r.engine.ReconnectMCPServer(ctx, name)
}

// AuthorizeMCPServer runs the interactive OAuth sign-in for an HTTP MCP server
// (workspace.mcp.authorize) — opens the system browser, catches the loopback
// redirect, and connects on success. Delegates to the engine, which owns the
// sessions. The credentials live for the process only (re-prompt after restart).
func (r *Runtime) AuthorizeMCPServer(ctx context.Context, name string) error {
	return r.engine.AuthorizeMCPServer(ctx, name)
}

// ConfigureMCPServer upserts a server in the registry and applies it to the
// live connections: an enabled server is (re)dialed, a disabled one is dropped
// from the live set (it stays in the registry). A dial failure does not fail
// the call — the server is persisted and tracked "failed" (reconnectable); the
// connectivity feedback path is TestMCPServer.
func (r *Runtime) ConfigureMCPServer(ctx context.Context, srv mcpserver.Server) error {
	if err := srv.Validate(); err != nil {
		return err
	}
	if err := r.mcpRegistry.Configure(ctx, srv); err != nil {
		return err
	}
	return r.applyAndGate(ctx, srv)
}

// RemoveMCPServer deletes a server from the registry and drops it from the live
// connections.
func (r *Runtime) RemoveMCPServer(ctx context.Context, name string) error {
	if err := r.mcpRegistry.Remove(ctx, name); err != nil {
		return err
	}
	// Shrink the live set before the gating (the disable direction of the
	// applyAndGate rule): dropping tools can't expose a hidden one, but
	// shrinking the gating first would leave the about-to-be-dropped tools
	// briefly live and ungated.
	r.engine.RemoveMCPServer(ctx, name)
	return r.refreshMCPGating(ctx)
}

// SetMCPServerEnabled flips a server's enablement in the registry and applies
// it to the live connections (enable → dial, disable → drop).
func (r *Runtime) SetMCPServerEnabled(ctx context.Context, name string, enabled bool) error {
	if err := r.mcpRegistry.SetEnabled(ctx, name, enabled); err != nil {
		return err
	}
	srv, ok, err := r.mcpRegistry.Get(ctx, name)
	if err != nil || !ok {
		return err
	}
	return r.applyAndGate(ctx, srv)
}

// applyAndGate reflects a just-persisted registry entry into BOTH the live tool
// set (engine) and the gating sets (atomic cell), ordered so a tool that should
// be hidden is never momentarily visible to the model. The two are read together
// at tool-resolution time but published here in two steps, so the order matters
// by direction:
//   - enabling: the server's tools are about to APPEAR, so the gating that hides
//     some of them must publish first (refresh → apply);
//   - disabling: the tools are about to be DROPPED, so the live set shrinks first,
//     then the gating (apply → refresh).
//
// Either reversal would leave a window where a disabled tool is live but ungated.
// The caller has already mutated the registry, so refreshMCPGating reads the new
// gating lists.
func (r *Runtime) applyAndGate(ctx context.Context, srv mcpserver.Server) error {
	if srv.Enabled {
		if err := r.refreshMCPGating(ctx); err != nil {
			return err
		}
		r.applyMCPServer(ctx, srv)
		return nil
	}
	r.applyMCPServer(ctx, srv)
	return r.refreshMCPGating(ctx)
}

// TestMCPServer dials srv with a throwaway client and proves its tools list —
// a connection test that touches neither the registry nor the live set, EXCEPT
// it reuses an active OAuth sign-in for the same-named server (so an authorized
// OAuth server tests as connected, not "unauthorized"). Returns the dial /
// tools-list error, or nil on success.
func (r *Runtime) TestMCPServer(ctx context.Context, srv mcpserver.Server) error {
	if err := srv.Validate(); err != nil {
		return err
	}
	return r.engine.ProbeMCPServer(ctx, configFromServer(srv))
}

// MCPTools lists tools advertised by the connected MCP servers (scoped to
// server when non-empty) for workspace.mcp.listTools. Delegates to the
// engine, which holds the dialed sessions.
func (r *Runtime) MCPTools(ctx context.Context, server string) ([]kernel.MCPToolInfo, error) {
	return r.engine.MCPTools(ctx, server)
}

// applyMCPServer reflects a registry entry into the live connections: enabled →
// (re)dial, disabled → drop. The dial error is intentionally swallowed (status
// surfaces it); see ConfigureMCPServer.
func (r *Runtime) applyMCPServer(ctx context.Context, srv mcpserver.Server) {
	if srv.Enabled {
		_ = r.engine.ConfigureMCPServer(ctx, configFromServer(srv))
		return
	}
	r.engine.RemoveMCPServer(ctx, srv.Name)
}
