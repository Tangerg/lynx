package integrations

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

const mcpReconcileTimeout = 30 * time.Second

// errClosed reports that a post-commit reconcile / background task could not be
// launched because the component is shutting down.
var errClosed = errors.New("integrations: closed")

// errMCPConnectionUnavailable reports an incomplete coordinator assembly at the
// use-case boundary instead of letting a detached task fail asynchronously.
var errMCPConnectionUnavailable = errors.New("integrations: MCP connection service is unavailable")

// MCP-server registry orchestration: the coordinator owns both the persisted
// registry (mcpserver.Registry) and the live connection pool, so a
// configure/remove/enable both persists and applies to the live tool set in one
// place. Registry entries are projected to dial-level descriptors only at the
// live-connection boundary.

// ListMCPRegisteredServers returns the persisted MCP-server registry entries,
// distinct from the live connection statuses returned by MCPServerStatuses.
func (c *Coordinator) ListMCPRegisteredServers(ctx context.Context) ([]mcpserver.Server, error) {
	return c.mcpRegistry.List(ctx)
}

// MCPServerStatuses returns the per-server connection state of every configured
// MCP server (connected and boot-failed alike) for mcp.servers.list.
func (c *Coordinator) MCPServerStatuses() []mcpserver.ConnectionStatus {
	if c.mcpStatusReader == nil {
		return nil
	}
	return c.mcpStatusReader.Statuses()
}

// MCPRegisteredServer returns one persisted MCP-server registry entry.
func (c *Coordinator) MCPRegisteredServer(ctx context.Context, name string) (mcpserver.Server, bool, error) {
	return c.mcpRegistry.Get(ctx, name)
}

// ReconnectMCPServer re-dials a configured MCP server and hot-swaps the live tool
// set (mcp.servers.reconnect). Fire-and-forget: the name is validated
// synchronously (unknown → [mcpserver.ErrUnknownServer]), then the dial runs on
// the component task group with connecting → settled status published for the
// workspace stream, so a returning RPC does not abort it while shutdown still can.
func (c *Coordinator) ReconnectMCPServer(ctx context.Context, name string) error {
	return c.startMCPConnection(ctx, name, func(ctx context.Context) error {
		return c.mcpConnectionCommands.Reconnect(ctx, name)
	})
}

// AuthorizeMCPServer runs the interactive OAuth sign-in for an HTTP MCP server
// (mcp.servers.authorize) — opens the system browser, catches the loopback
// redirect, and connects on success. Fire-and-forget like reconnect; the
// credentials live for the process only (re-prompt after restart).
func (c *Coordinator) AuthorizeMCPServer(ctx context.Context, name string) error {
	return c.startMCPConnection(ctx, name, func(ctx context.Context) error {
		return c.mcpConnectionCommands.Authorize(ctx, name)
	})
}

// startMCPConnection validates the server exists, then runs dial on the
// component task group — detached from the caller's cancellation (so a returning
// RPC cannot abort it) but keeping its trace values and canceled + joined by
// Close. It enters the application mutation order only for the pre/post registry
// checks and status publication; the dial itself runs outside that global
// critical section. The live adapter's per-server generation makes a concurrent
// configure/remove supersede stale dial completion, while unrelated servers can
// connect in parallel. The task's context scopes both registry reads and dial.
// Returns [errMCPConnectionUnavailable] when the coordinator lacks a required
// connection dependency, [mcpserver.ErrUnknownServer] for an unknown name (the
// delivery layer maps it to invalid_params), or [errClosed] during shutdown.
func (c *Coordinator) startMCPConnection(ctx context.Context, name string, dial func(context.Context) error) error {
	if c.mcpRegistry == nil || c.mcpStatusReader == nil || c.mcpConnectionCommands == nil {
		return errMCPConnectionUnavailable
	}
	if !c.mcpServerKnown(name) {
		return mcpserver.ErrUnknownServer
	}
	if !c.tasks.Start(ctx, func(ctx context.Context) {
		c.mcpMutationMu.Lock()
		srv, ok, err := c.mcpRegistry.Get(ctx, name)
		if err != nil {
			c.mcpMutationMu.Unlock()
			recordMCPConnectionError(ctx, fmt.Errorf("integrations: read MCP server %q before connection: %w", name, err))
			return
		}
		if !ok || !srv.Enabled {
			c.mcpMutationMu.Unlock()
			return
		}
		c.notifyMCPStatus(ctx, name, true)
		c.mcpMutationMu.Unlock()

		// Interactive OAuth may wait minutes for a human. The live connection
		// adapter owns per-server generation/cancellation, so no application-wide
		// mutation lock is held while dialing. A configure/remove can supersede it
		// immediately; stale adapter completion cannot swap itself back in.
		_ = dial(ctx)

		c.mcpMutationMu.Lock()
		defer c.mcpMutationMu.Unlock()
		srv, ok, err = c.mcpRegistry.Get(ctx, name)
		if err != nil {
			recordMCPConnectionError(ctx, fmt.Errorf("integrations: read MCP server %q after connection: %w", name, err))
			return
		}
		if !ok || !srv.Enabled {
			// Defensive projection cleanup for adapters that cannot cancel a stale
			// operation themselves. The production adapter rejects stale generations,
			// so this is normally an idempotent no-op.
			if c.mcpRegistryCommands != nil {
				c.mcpRegistryCommands.Remove(ctx, name)
			}
			return
		}
		c.notifyMCPStatus(ctx, name, false)
	}) {
		return errClosed
	}
	return nil
}

func recordMCPConnectionError(ctx context.Context, err error) {
	if err != nil {
		trace.SpanFromContext(ctx).RecordError(err)
	}
}

// mcpServerKnown reports whether name is a tracked MCP server (a configured
// server appears in the live statuses even when its last dial failed).
func (c *Coordinator) mcpServerKnown(name string) bool {
	if c.mcpStatusReader == nil {
		return false
	}
	for _, st := range c.mcpStatusReader.Statuses() {
		if st.Name == name {
			return true
		}
	}
	return false
}

func (c *Coordinator) notifyMCPStatus(ctx context.Context, name string, connecting bool) {
	if c.mcpStatus != nil {
		c.mcpStatus(ctx, name, connecting)
	}
}

// ConfigureMCPServer upserts a server in the registry and applies it to the live
// connections: an enabled server is (re)dialed, a disabled one is dropped from
// the live set (it stays in the registry). A dial failure does not fail the call
// — the server is persisted and tracked "failed" (reconnectable); the
// connectivity feedback path is TestMCPServer.
func (c *Coordinator) ConfigureMCPServer(ctx context.Context, srv mcpserver.Server) error {
	if err := srv.Validate(); err != nil {
		return err
	}
	requestCtx, ownerCtx, release, err := c.beginMCPWrite(ctx)
	if err != nil {
		return err
	}
	defer release()
	if err := c.mcpRegistry.Configure(requestCtx, srv); err != nil {
		return err
	}
	reconcileCtx, cancel := context.WithTimeout(ownerCtx, mcpReconcileTimeout)
	defer cancel()
	return c.applyMCPRegistryChange(reconcileCtx, srv)
}

// RemoveMCPServer deletes a server from the registry and drops it from the live
// connections.
func (c *Coordinator) RemoveMCPServer(ctx context.Context, name string) error {
	requestCtx, ownerCtx, release, err := c.beginMCPWrite(ctx)
	if err != nil {
		return err
	}
	defer release()
	if err := c.mcpRegistry.Remove(requestCtx, name); err != nil {
		return err
	}
	reconcileCtx, cancel := context.WithTimeout(ownerCtx, mcpReconcileTimeout)
	defer cancel()
	// Shrink the live set before publishing the new policy: dropping tools can't
	// expose a hidden one, but publishing first would leave the about-to-be-dropped
	// tools briefly live under the wrong policy.
	if c.mcpRegistryCommands != nil {
		c.mcpRegistryCommands.Remove(reconcileCtx, name)
	}
	return c.refreshMCPToolPolicy(reconcileCtx)
}

// SetMCPServerEnabled flips a server's enablement in the registry and applies it
// to the live connections (enable → dial, disable → drop).
func (c *Coordinator) SetMCPServerEnabled(ctx context.Context, name string, enabled bool) error {
	requestCtx, ownerCtx, release, err := c.beginMCPWrite(ctx)
	if err != nil {
		return err
	}
	defer release()
	if err := c.mcpRegistry.SetEnabled(requestCtx, name, enabled); err != nil {
		return err
	}
	reconcileCtx, cancel := context.WithTimeout(ownerCtx, mcpReconcileTimeout)
	defer cancel()
	srv, ok, err := c.mcpRegistry.Get(reconcileCtx, name)
	if err != nil {
		return err
	}
	if !ok {
		return mcpserver.ErrUnknownServer
	}
	return c.applyMCPRegistryChange(reconcileCtx, srv)
}

// beginMCPMutation gives a write both scopes it needs: requestCtx is canceled by
// the caller or component shutdown and is used until the durable registry commit;
// ownerCtx ignores caller cancellation but is still canceled by shutdown, so
// post-commit live/policy reconciliation cannot be abandoned by a dropped
// connection or escape component shutdown.
func (c *Coordinator) beginMCPMutation(parent context.Context) (
	requestCtx context.Context,
	ownerCtx context.Context,
	finish func(),
	err error,
) {
	ownerCtx, releaseOwner, ok := c.tasks.Attach(parent)
	if !ok {
		return nil, nil, nil, errClosed
	}
	requestCtx, releaseRequest, ok := c.tasks.AttachLinked(parent)
	if !ok {
		releaseOwner()
		return nil, nil, nil, errClosed
	}
	return requestCtx, ownerCtx, func() {
		releaseRequest()
		releaseOwner()
	}, nil
}

// beginMCPWrite acquires both mutation scopes plus the serialization lock and
// returns a single release the caller must defer. requestCtx is caller-cancelable
// (used through the durable registry commit); ownerCtx survives caller cancel but
// not component shutdown (used for post-commit live/policy reconciliation).
// release unlocks then tears the scopes down — the acquire order reversed — so the
// lock/ctx/teardown protocol lives in one place across the write methods. err is
// non-nil when the component is shutting down or the caller ctx is already dead.
func (c *Coordinator) beginMCPWrite(ctx context.Context) (requestCtx, ownerCtx context.Context, release func(), err error) {
	requestCtx, ownerCtx, finish, err := c.beginMCPMutation(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	c.mcpMutationMu.Lock()
	if err := requestCtx.Err(); err != nil {
		c.mcpMutationMu.Unlock()
		finish()
		return nil, nil, nil, err
	}
	return requestCtx, ownerCtx, func() {
		c.mcpMutationMu.Unlock()
		finish()
	}, nil
}

// applyMCPRegistryChange reflects a persisted registry entry into the live tool
// set and the policy snapshot. Publication order keeps disabled tools from
// becoming momentarily visible:
//   - enabling publishes policy before adding tools;
//   - disabling removes tools before publishing policy.
//
// Either reversal would leave a window where a disabled tool is live under the
// wrong policy. The caller has already mutated the registry, so
// refreshMCPToolPolicy reads the new policy inputs.
func (c *Coordinator) applyMCPRegistryChange(ctx context.Context, srv mcpserver.Server) error {
	if srv.Enabled {
		if err := c.refreshMCPToolPolicy(ctx); err != nil {
			return err
		}
		c.applyMCPServer(ctx, srv)
		return nil
	}
	c.applyMCPServer(ctx, srv)
	return c.refreshMCPToolPolicy(ctx)
}

// TestMCPServer dials srv with a throwaway client and proves its tools list — a
// connection test that touches neither the registry nor the live set, EXCEPT it
// reuses an active OAuth sign-in for the same-named server (so an authorized
// OAuth server tests as connected, not "unauthorized"). Returns the dial /
// tools-list error, or nil on success.
func (c *Coordinator) TestMCPServer(ctx context.Context, srv mcpserver.Server) error {
	if err := srv.Validate(); err != nil {
		return err
	}
	if c.mcpRegistryCommands == nil {
		return mcpserver.ErrUnknownServer
	}
	return c.mcpRegistryCommands.Probe(ctx, mcpserver.ConfigFromServer(srv))
}

// MCPTools lists tools advertised by the connected MCP servers (scoped to server
// when non-empty) for mcp.tools.list.
func (c *Coordinator) MCPTools(ctx context.Context, server string) ([]mcpserver.ToolInfo, error) {
	if c.mcpToolCatalog == nil {
		return nil, nil
	}
	return c.mcpToolCatalog.Tools(ctx, server)
}

// applyMCPServer reflects a registry entry into the live connections: enabled →
// (re)dial, disabled → drop. The dial error is intentionally swallowed (status
// surfaces it); see ConfigureMCPServer.
func (c *Coordinator) applyMCPServer(ctx context.Context, srv mcpserver.Server) {
	if c.mcpRegistryCommands == nil {
		return
	}
	if srv.Enabled {
		_ = c.mcpRegistryCommands.Configure(ctx, mcpserver.ConfigFromServer(srv))
		return
	}
	c.mcpRegistryCommands.Remove(ctx, srv.Name)
}

// refreshMCPToolPolicy atomically publishes the policy derived from the
// just-mutated registry for the next tool resolution and approval decision.
func (c *Coordinator) refreshMCPToolPolicy(ctx context.Context) error {
	servers, err := c.mcpRegistry.List(ctx)
	if err != nil {
		return err
	}
	policy := mcpserver.NewToolPolicy(servers)
	c.mcpPolicy.Store(&policy)
	return nil
}
