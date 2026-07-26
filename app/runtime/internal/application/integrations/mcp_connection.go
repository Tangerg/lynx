package integrations

import (
	"context"
	"fmt"
	"sync"

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
	registryCtx := context.Background()
	if ctx != nil {
		registryCtx = context.WithoutCancel(ctx)
	}
	srv, ok, err := c.mcpRegistry.Get(registryCtx, name)
	if err != nil {
		return fmt.Errorf("integrations: read MCP server %q: %w", name, err)
	}
	if !ok {
		return ErrUnknownMCPServer
	}
	if !srv.Enabled {
		return ErrMCPServerDisabled
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
		if !c.currentMCPDial(name, operation) {
			c.mcpMutationMu.Unlock()
			return
		}
		connecting := c.prepareMCPStatus(MCPServerStatus{
			Name:  name,
			Known: true,
			State: mcpserver.ConnectionConnecting,
		})
		c.mcpMutationMu.Unlock()
		c.publishMCPStatus(connecting)

		// Interactive OAuth may wait minutes for a human. The live connection
		// adapter owns per-server generation/cancellation, so no application-wide
		// mutation lock is held while dialing. A configure/remove can supersede it
		// immediately; stale adapter completion cannot swap itself back in.
		if err := connect(ctx); err != nil && ctx.Err() == nil {
			recordMCPConnectionError(ctx, fmt.Errorf("integrations: connect MCP server %q: %w", name, err))
		}
		if ctx.Err() != nil {
			return
		}

		status := c.MCPServerStatus(ctx, name)
		c.mcpMutationMu.Lock()
		srv, ok, err = c.mcpRegistry.Get(ctx, name)
		if err != nil {
			c.mcpMutationMu.Unlock()
			recordMCPConnectionError(ctx, fmt.Errorf("integrations: read MCP server %q after connection: %w", name, err))
			return
		}
		if !ok || !srv.Enabled || !c.currentMCPDial(name, operation) {
			c.mcpMutationMu.Unlock()
			return
		}
		settled := c.prepareMCPStatus(status)
		c.mcpMutationMu.Unlock()
		c.publishMCPStatus(settled)
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
// canceling its context; adapters must honor ctx while dialing and reject a
// stale completion through their per-server generation check.
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

func (c *Coordinator) currentMCPDial(name string, dial *mcpDial) bool {
	c.mcpDialMu.Lock()
	defer c.mcpDialMu.Unlock()
	return c.mcpDials[name] == dial
}

func recordMCPConnectionError(ctx context.Context, err error) {
	if err != nil {
		trace.SpanFromContext(ctx).RecordError(err)
	}
}

type mcpStatusEvent struct {
	sequence uint64
	status   MCPServerStatus
}

type mcpStatusQueue struct {
	mu       sync.Mutex
	next     uint64
	pending  map[uint64]MCPServerStatus
	draining bool
	sink     func(MCPServerStatus)
}

func newMCPStatusQueue(sink func(MCPServerStatus)) *mcpStatusQueue {
	return &mcpStatusQueue{
		next:    1,
		pending: make(map[uint64]MCPServerStatus),
		sink:    sink,
	}
}

// prepareMCPStatus is called while mcpMutationMu is held. The sequence lets
// lock-free callback delivery retain the exact mutation order.
func (c *Coordinator) prepareMCPStatus(status MCPServerStatus) mcpStatusEvent {
	c.mcpStatusSequence++
	return mcpStatusEvent{sequence: c.mcpStatusSequence, status: status}
}

func (c *Coordinator) publishMCPStatus(event mcpStatusEvent) {
	c.mcpStatusQueue.publish(event)
}

func (q *mcpStatusQueue) publish(event mcpStatusEvent) {
	if q == nil || q.sink == nil || event.sequence == 0 {
		return
	}
	q.mu.Lock()
	q.pending[event.sequence] = event.status
	if q.draining {
		q.mu.Unlock()
		return
	}
	q.draining = true
	q.mu.Unlock()

	for {
		q.mu.Lock()
		status, ok := q.pending[q.next]
		if !ok {
			q.draining = false
			q.mu.Unlock()
			return
		}
		delete(q.pending, q.next)
		q.next++
		q.mu.Unlock()
		q.sink(status)
	}
}
