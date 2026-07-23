package integrations

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

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
		return ErrUnknownMCPServer
	}
	return c.dispatchMCPConnection(ctx, name, dial)
}

// dispatchMCPConnection runs a live (re)dial on the component task group, detached
// from the caller's cancellation. It enters the mutation order only for the
// pre/post registry checks and status publication; the dial itself runs OUTSIDE
// that global critical section, so a slow endpoint cannot freeze the control
// plane, and the registry re-read lets a concurrent configure/remove supersede a
// stale completion. A caller that already holds mcpMutationMu may invoke this: the
// spawned task blocks on the lock until that caller releases it, then proceeds —
// which is exactly how the registry-write methods dispatch their live dial without
// holding the lock across the network handshake. Returns errClosed only when the
// task group is shutting down.
func (c *Coordinator) dispatchMCPConnection(ctx context.Context, name string, connect func(context.Context) error) error {
	ownerCtx, releaseOwner, ok := c.tasks.Attach(ctx)
	if !ok {
		return errClosed
	}
	dialCtx, operation := c.replaceMCPDial(ownerCtx, name)
	if !c.tasks.StartLinked(dialCtx, func(ctx context.Context) {
		defer releaseOwner()
		defer c.clearMCPDial(name, operation)
		if err := ctx.Err(); err != nil {
			return
		}
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
		_ = connect(ctx)

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
		operation.cancel()
		c.clearMCPDial(name, operation)
		releaseOwner()
		return errClosed
	}
	return nil
}

// replaceMCPDial gives each server exactly one current connection operation.
// A registry mutation, reconnect, or authorize supersedes the previous dial by
// canceling its context; adapters must honor ctx while dialing. The generation
// check after a dial remains a defensive cleanup for adapters whose transport
// cannot stop synchronously.
func (c *Coordinator) replaceMCPDial(ctx context.Context, name string) (context.Context, *mcpDial) {
	dialCtx, cancel := context.WithCancel(ctx)
	dial := &mcpDial{cancel: cancel}
	c.mcpDialMu.Lock()
	if previous := c.mcpDials[name]; previous != nil {
		previous.cancel()
	}
	c.mcpDials[name] = dial
	c.mcpDialMu.Unlock()
	return dialCtx, dial
}

func (c *Coordinator) cancelMCPDial(name string) {
	c.mcpDialMu.Lock()
	if dial := c.mcpDials[name]; dial != nil {
		dial.cancel()
		delete(c.mcpDials, name)
	}
	c.mcpDialMu.Unlock()
}

func (c *Coordinator) clearMCPDial(name string, dial *mcpDial) {
	c.mcpDialMu.Lock()
	if c.mcpDials[name] == dial {
		delete(c.mcpDials, name)
	}
	c.mcpDialMu.Unlock()
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
		if connecting {
			c.mcpStatus(MCPServerStatus{Name: name, Known: true, State: mcpserver.ConnectionConnecting})
			return
		}
		c.mcpStatus(c.MCPServerStatus(ctx, name))
	}
}
