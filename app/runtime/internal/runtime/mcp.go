package runtime

import (
	"context"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/toolport"
)

const mcpReconcileTimeout = 30 * time.Second

// MCP-server registry orchestration: the runtime owns both the persisted
// registry (mcpserver.Registry) and the live connection ports, so a
// configure/remove/enable both persists and applies to the live tool set in one
// place. Registry entries are projected to dial-level descriptors only at the
// live-connection boundary.

type mcpServerList interface {
	List(ctx context.Context) ([]mcpserver.Server, error)
}

type mcpLiveStatusReader interface {
	MCPServerStatuses() []toolport.MCPServerStatus
}

type mcpLiveToolCatalog interface {
	MCPTools(ctx context.Context, server string) ([]toolport.MCPToolInfo, error)
}

type mcpLiveConnectionCommands interface {
	ReconnectMCPServer(ctx context.Context, name string) error
	AuthorizeMCPServer(ctx context.Context, name string) error
}

type mcpLiveRegistryCommands interface {
	ProbeMCPServer(ctx context.Context, cfg toolport.MCPServerConfig) error
	ConfigureMCPServer(ctx context.Context, cfg toolport.MCPServerConfig) error
	RemoveMCPServer(ctx context.Context, name string)
}

// ListMCPRegisteredServers returns the persisted MCP-server registry entries,
// distinct from the live connection statuses returned by MCPServerStatuses.
func (r *Runtime) ListMCPRegisteredServers(ctx context.Context) ([]mcpserver.Server, error) {
	return r.mcpRegistry.List(ctx)
}

// MCPServerStatuses returns the per-server connection state of every
// configured MCP server (connected and boot-failed alike) for
// workspace.mcp.listServers. Delegates to the live MCP status port.
func (r *Runtime) MCPServerStatuses() []toolport.MCPServerStatus {
	if r.mcpLiveStatus == nil {
		return nil
	}
	return r.mcpLiveStatus.MCPServerStatuses()
}

// MCPRegisteredServer returns one persisted MCP-server registry entry.
func (r *Runtime) MCPRegisteredServer(ctx context.Context, name string) (mcpserver.Server, bool, error) {
	return r.mcpRegistry.Get(ctx, name)
}

// ReconnectMCPServer re-dials a configured MCP server and hot-swaps the live
// tool set (workspace.mcp.reconnect). Delegates to the live MCP connection port.
func (r *Runtime) ReconnectMCPServer(ctx context.Context, name string) error {
	if r.mcpLiveConnections == nil {
		return toolport.ErrUnknownMCPServer
	}
	return r.mcpLiveConnections.ReconnectMCPServer(ctx, name)
}

// AuthorizeMCPServer runs the interactive OAuth sign-in for an HTTP MCP server
// (workspace.mcp.authorize) — opens the system browser, catches the loopback
// redirect, and connects on success. Delegates to the live MCP connection port.
// The credentials live for the process only (re-prompt after restart).
func (r *Runtime) AuthorizeMCPServer(ctx context.Context, name string) error {
	if r.mcpLiveConnections == nil {
		return toolport.ErrUnknownMCPServer
	}
	return r.mcpLiveConnections.AuthorizeMCPServer(ctx, name)
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
	requestCtx, ownerCtx, finish, err := r.beginMCPMutation(ctx)
	if err != nil {
		return err
	}
	defer finish()
	r.mcpMutationMu.Lock()
	defer r.mcpMutationMu.Unlock()
	if err := requestCtx.Err(); err != nil {
		return err
	}
	if err := r.mcpRegistry.Configure(requestCtx, srv); err != nil {
		return err
	}
	reconcileCtx, cancel := context.WithTimeout(ownerCtx, mcpReconcileTimeout)
	defer cancel()
	return r.applyMCPRegistryChange(reconcileCtx, srv)
}

// RemoveMCPServer deletes a server from the registry and drops it from the live
// connections.
func (r *Runtime) RemoveMCPServer(ctx context.Context, name string) error {
	requestCtx, ownerCtx, finish, err := r.beginMCPMutation(ctx)
	if err != nil {
		return err
	}
	defer finish()
	r.mcpMutationMu.Lock()
	defer r.mcpMutationMu.Unlock()
	if err := requestCtx.Err(); err != nil {
		return err
	}
	if err := r.mcpRegistry.Remove(requestCtx, name); err != nil {
		return err
	}
	reconcileCtx, cancel := context.WithTimeout(ownerCtx, mcpReconcileTimeout)
	defer cancel()
	// Shrink the live set before publishing the new policy: dropping tools can't
	// expose a hidden one, but publishing first would leave the
	// about-to-be-dropped tools briefly live under the wrong policy.
	if r.mcpLiveRegistry != nil {
		r.mcpLiveRegistry.RemoveMCPServer(reconcileCtx, name)
	}
	return r.refreshMCPToolPolicy(reconcileCtx)
}

// SetMCPServerEnabled flips a server's enablement in the registry and applies
// it to the live connections (enable → dial, disable → drop).
func (r *Runtime) SetMCPServerEnabled(ctx context.Context, name string, enabled bool) error {
	requestCtx, ownerCtx, finish, err := r.beginMCPMutation(ctx)
	if err != nil {
		return err
	}
	defer finish()
	r.mcpMutationMu.Lock()
	defer r.mcpMutationMu.Unlock()
	if err := requestCtx.Err(); err != nil {
		return err
	}
	if err := r.mcpRegistry.SetEnabled(requestCtx, name, enabled); err != nil {
		return err
	}
	reconcileCtx, cancel := context.WithTimeout(ownerCtx, mcpReconcileTimeout)
	defer cancel()
	srv, ok, err := r.mcpRegistry.Get(reconcileCtx, name)
	if err != nil || !ok {
		return err
	}
	return r.applyMCPRegistryChange(reconcileCtx, srv)
}

// beginMCPMutation gives a write both scopes it needs: requestCtx is canceled
// by the caller or Runtime.Close and is used until the durable registry commit;
// ownerCtx ignores caller cancellation but is still canceled by Runtime.Close,
// so post-commit live/policy reconciliation cannot be abandoned by a dropped
// connection or escape component shutdown.
func (r *Runtime) beginMCPMutation(parent context.Context) (
	requestCtx context.Context,
	ownerCtx context.Context,
	finish func(),
	err error,
) {
	ownerCtx, releaseOwner, ok := r.tasks.Attach(parent)
	if !ok {
		return nil, nil, nil, errRuntimeClosed
	}
	requestCtx, releaseRequest, ok := r.tasks.AttachLinked(parent)
	if !ok {
		releaseOwner()
		return nil, nil, nil, errRuntimeClosed
	}
	return requestCtx, ownerCtx, func() {
		releaseRequest()
		releaseOwner()
	}, nil
}

// applyMCPRegistryChange reflects a persisted registry entry into the live tool
// set and the policy snapshot. Publication order keeps disabled tools from
// becoming momentarily visible:
//   - enabling publishes policy before adding tools;
//   - disabling removes tools before publishing policy.
//
// Either reversal would leave a window where a disabled tool is live under the
// wrong policy.
// The caller has already mutated the registry, so refreshMCPToolPolicy reads
// the new policy inputs.
func (r *Runtime) applyMCPRegistryChange(ctx context.Context, srv mcpserver.Server) error {
	if srv.Enabled {
		if err := r.refreshMCPToolPolicy(ctx); err != nil {
			return err
		}
		r.applyMCPServer(ctx, srv)
		return nil
	}
	r.applyMCPServer(ctx, srv)
	return r.refreshMCPToolPolicy(ctx)
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
	if r.mcpLiveRegistry == nil {
		return toolport.ErrUnknownMCPServer
	}
	return r.mcpLiveRegistry.ProbeMCPServer(ctx, configFromServer(srv))
}

// MCPTools lists tools advertised by the connected MCP servers (scoped to
// server when non-empty) for workspace.mcp.listTools. Delegates to the live MCP
// tool catalog port.
func (r *Runtime) MCPTools(ctx context.Context, server string) ([]toolport.MCPToolInfo, error) {
	if r.mcpLiveTools == nil {
		return nil, nil
	}
	return r.mcpLiveTools.MCPTools(ctx, server)
}

// applyMCPServer reflects a registry entry into the live connections: enabled →
// (re)dial, disabled → drop. The dial error is intentionally swallowed (status
// surfaces it); see ConfigureMCPServer.
func (r *Runtime) applyMCPServer(ctx context.Context, srv mcpserver.Server) {
	if r.mcpLiveRegistry == nil {
		return
	}
	if srv.Enabled {
		_ = r.mcpLiveRegistry.ConfigureMCPServer(ctx, configFromServer(srv))
		return
	}
	r.mcpLiveRegistry.RemoveMCPServer(ctx, srv.Name)
}
