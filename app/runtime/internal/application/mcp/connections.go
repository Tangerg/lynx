package mcp

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/scope/app/runtime/internal/domain/mcpserver"
)

// ReconnectServer re-dials a configured MCP server and hot-swaps the live tool
// set (mcp.servers.reconnect). Fire-and-forget: the name is validated
// synchronously (unknown → [ErrUnknownServer], disabled →
// [ErrServerDisabled]), then the dial runs on
// the component task group with connecting → settled status published for the
// status observers, so the initiating request does not abort it while shutdown
// still can.
func (c *Coordinator) ReconnectServer(ctx context.Context, name string) error {
	return c.startConnection(ctx, name, func(ctx context.Context) error {
		return c.connectionControl.Reconnect(ctx, name)
	})
}

// startConnection validates the server exists, then runs dial on the
// component task group — detached from the caller's cancellation but keeping
// its trace values and canceled + joined by
// Close. It enters the application mutation order only for the pre/post registry
// checks and status publication; the dial itself runs outside that global
// critical section. The connection command's per-server generation makes a
// concurrent configure/remove supersede stale dial completion, while unrelated
// servers can connect in parallel. The task's context scopes both registry reads
// and dial.
// Returns [errConnectionUnavailable] when the coordinator lacks a required
// connection dependency, [ErrUnknownServer] or [ErrServerDisabled] when
// durable state refuses the command, or [errClosed] during shutdown.
func (c *Coordinator) startConnection(ctx context.Context, name string, dial func(context.Context) error) error {
	if _, err := c.connectionTarget(ctx, name); err != nil {
		return err
	}
	return c.dispatchConnection(ctx, name, dial, true, nil, nil)
}

func (c *Coordinator) connectionTarget(ctx context.Context, name string) (mcpserver.Server, error) {
	if c.registry == nil || c.statusReader == nil || c.connectionControl == nil {
		return mcpserver.Server{}, errConnectionUnavailable
	}
	if ctx == nil {
		return mcpserver.Server{}, errors.New("mcp: connection context is required")
	}
	registryCtx := context.WithoutCancel(ctx)
	srv, ok, err := c.registry.Get(registryCtx, name)
	if err != nil {
		return mcpserver.Server{}, fmt.Errorf("mcp: read MCP server %q: %w", name, err)
	}
	if !ok {
		return mcpserver.Server{}, ErrUnknownServer
	}
	if !srv.Enabled {
		return mcpserver.Server{}, ErrServerDisabled
	}
	return srv, nil
}

type connectionOutcome uint8

const (
	connectionSucceeded connectionOutcome = iota + 1
	connectionFailed
	connectionCanceled
)

// dispatchConnection runs a live (re)dial on the component task group, detached
// from the caller's cancellation. It enters the mutation order only for the
// pre/post registry checks and status publication; the dial itself runs OUTSIDE
// that global critical section, so a slow endpoint cannot freeze the control
// plane, and the registry re-read lets a concurrent configure/remove supersede a
// stale completion. A caller that already holds mutationMu may invoke this: the
// spawned task blocks on the lock until that caller releases it, then proceeds —
// which is exactly how the registry-write methods dispatch their live dial without
// holding the lock across the network handshake. When completed is non-nil, an
// admitted task calls it exactly once after it reaches succeeded, failed, or
// canceled; admission failure calls nothing. Returns errClosed only when the task
// group is shutting down.
func (c *Coordinator) dispatchConnection(
	ctx context.Context,
	name string,
	connect func(context.Context) error,
	publishConnecting bool,
	start <-chan struct{},
	completed func(connectionOutcome),
) error {
	ownerCtx, releaseOwner, ok := c.tasks.Attach(ctx)
	if !ok {
		return errClosed
	}
	dialCtx, operation := c.replaceDial(ownerCtx, name)
	if !c.tasks.StartLinked(dialCtx, func(ctx context.Context) {
		outcome := connectionCanceled
		if completed != nil {
			defer func() { completed(outcome) }()
		}
		defer releaseOwner()
		defer c.clearDial(name, operation)
		if start != nil {
			select {
			case <-start:
			case <-ctx.Done():
				return
			}
		}
		if err := ctx.Err(); err != nil {
			return
		}
		c.mutationMu.Lock()
		srv, ok, err := c.registry.Get(ctx, name)
		if err != nil {
			c.mutationMu.Unlock()
			recordConnectionError(ctx, fmt.Errorf("mcp: read MCP server %q before connection: %w", name, err))
			outcome = connectionFailed
			return
		}
		if !ok || !srv.Enabled {
			c.mutationMu.Unlock()
			return
		}
		if !c.currentDial(name, operation) {
			c.mutationMu.Unlock()
			return
		}
		var connecting statusEvent
		if publishConnecting {
			connecting = c.prepareStatus(ServerStatus{
				Name:  name,
				Known: true,
				State: mcpserver.ConnectionConnecting,
			})
		}
		c.mutationMu.Unlock()
		c.publishStatus(connecting)

		// Interactive OAuth may wait minutes for a human. The connection command
		// owns per-server generation and cancellation, so no application-wide
		// mutation lock is held while dialing. A configure/remove can supersede it
		// immediately; stale completion cannot swap itself back in.
		connectionErr := connect(ctx)
		if connectionErr != nil && ctx.Err() == nil {
			recordConnectionError(ctx, fmt.Errorf("mcp: connect MCP server %q: %w", name, connectionErr))
		}
		if ctx.Err() != nil {
			return
		}

		status := c.liveStatus(name)
		c.mutationMu.Lock()
		srv, ok, err = c.registry.Get(ctx, name)
		if err != nil {
			c.mutationMu.Unlock()
			recordConnectionError(ctx, fmt.Errorf("mcp: read MCP server %q after connection: %w", name, err))
			outcome = connectionFailed
			return
		}
		if !ok || !srv.Enabled || !c.currentDial(name, operation) {
			c.mutationMu.Unlock()
			return
		}
		settled := c.prepareStatus(status)
		c.mutationMu.Unlock()
		c.publishStatus(settled)
		if connectionErr != nil || status.State != mcpserver.ConnectionConnected {
			outcome = connectionFailed
			return
		}
		outcome = connectionSucceeded
	}) {
		operation.cancel()
		c.clearDial(name, operation)
		releaseOwner()
		return errClosed
	}
	return nil
}

// replaceDial gives each server exactly one current connection operation.
// A registry mutation, reconnect, or authorization attempt supersedes the previous dial by
// canceling its context; connection commands must honor ctx while dialing and
// reject a stale completion through their per-server generation check.
func (c *Coordinator) replaceDial(ctx context.Context, name string) (context.Context, *activeDial) {
	dialCtx, cancel := context.WithCancel(ctx)
	dial := &activeDial{cancel: cancel}
	c.dialMu.Lock()
	if previous := c.dials[name]; previous != nil {
		previous.cancel()
	}
	c.dials[name] = dial
	c.dialMu.Unlock()
	return dialCtx, dial
}

func (c *Coordinator) cancelDial(name string) {
	c.dialMu.Lock()
	if dial := c.dials[name]; dial != nil {
		dial.cancel()
		delete(c.dials, name)
	}
	c.dialMu.Unlock()
}

func (c *Coordinator) clearDial(name string, dial *activeDial) {
	c.dialMu.Lock()
	if c.dials[name] == dial {
		delete(c.dials, name)
	}
	c.dialMu.Unlock()
}

func (c *Coordinator) currentDial(name string, dial *activeDial) bool {
	c.dialMu.Lock()
	defer c.dialMu.Unlock()
	return c.dials[name] == dial
}

func recordConnectionError(ctx context.Context, err error) {
	if err != nil {
		trace.SpanFromContext(ctx).RecordError(err)
	}
}

type statusEvent struct {
	sequence uint64
	status   ServerStatus
}

type statusQueue struct {
	mu       sync.Mutex
	next     uint64
	pending  map[uint64]ServerStatus
	draining bool
	sink     func(ServerStatus)
}

func newStatusQueue(sink func(ServerStatus)) *statusQueue {
	return &statusQueue{
		next:    1,
		pending: make(map[uint64]ServerStatus),
		sink:    sink,
	}
}

// prepareStatus is called while mutationMu is held. The sequence lets
// lock-free callback publication retain the exact mutation order.
func (c *Coordinator) prepareStatus(status ServerStatus) statusEvent {
	c.statusSequence++
	return statusEvent{sequence: c.statusSequence, status: status}
}

func (c *Coordinator) publishStatus(event statusEvent) {
	c.statusQueue.publish(event)
}

func (s *statusQueue) publish(event statusEvent) {
	if s == nil || s.sink == nil || event.sequence == 0 {
		return
	}
	s.mu.Lock()
	s.pending[event.sequence] = event.status
	if s.draining {
		s.mu.Unlock()
		return
	}
	s.draining = true
	s.mu.Unlock()

	for {
		s.mu.Lock()
		status, ok := s.pending[s.next]
		if !ok {
			s.draining = false
			s.mu.Unlock()
			return
		}
		delete(s.pending, s.next)
		s.next++
		s.mu.Unlock()
		s.sink(status)
	}
}
