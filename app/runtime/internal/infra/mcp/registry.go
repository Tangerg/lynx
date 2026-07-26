package mcp

import (
	"context"
	"errors"
	"fmt"
	"slices"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/tools"
)

// target is a connected (name, session) pair snapshotted under the lock so the
// live tools/list RPCs can run outside it.
type target struct {
	name    string
	session *sdkmcp.ClientSession
}

// Statuses returns one cached entry per server attached to the live projection
// (connected and failed alike), in dial order. Nil-safe.
func (c *Connections) Statuses() []mcpserver.ConnectionStatus {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]mcpserver.ConnectionStatus, 0, len(c.servers))
	for _, ms := range c.servers {
		out = append(out, mcpserver.ConnectionStatus{
			Name:      ms.name(),
			State:     ms.state,
			ToolCount: len(ms.tools),
		})
	}
	return out
}

// Tools lists the tools advertised by the connected servers, scoped to server
// when non-empty. It queries each session's tools/list live, ordered by
// (server, tool name) as dialed. Nil-safe.
func (c *Connections) Tools(ctx context.Context, server string) ([]mcpserver.ToolInfo, error) {
	if c == nil {
		return nil, nil
	}
	// Snapshot the connected (name, session) pairs under the lock, then run the
	// live tools/list RPCs outside it — a slow upstream mustn't block reconnect
	// or status reads. A session closed by a racing reconnect just errors here.
	c.mu.Lock()
	var targets []target
	for _, ms := range c.servers {
		if ms.session == nil || (server != "" && ms.name() != server) {
			continue
		}
		targets = append(targets, target{ms.name(), ms.session})
	}
	c.mu.Unlock()

	var out []mcpserver.ToolInfo
	for _, t := range targets {
		for descriptor, err := range t.session.Tools(ctx, nil) {
			if err != nil {
				return nil, fmt.Errorf("mcp: list tools from server %q: %w", t.name, err)
			}
			schema, err := inputSchema(descriptor.InputSchema)
			if err != nil {
				return nil, fmt.Errorf(
					"mcp: decode input schema for tool %q from server %q: %w",
					descriptor.Name,
					t.name,
					err,
				)
			}
			out = append(out, mcpserver.ToolInfo{
				Server:      t.name,
				Name:        descriptor.Name,
				Description: descriptor.Description,
				InputSchema: schema,
			})
		}
	}
	return out, nil
}

// Detach removes a server from the live projection and starts retiring its
// session. Session teardown remains owned by Connections and is joined by
// Shutdown; it never delays the application control-plane mutation.
func (c *Connections) Detach(name string) error {
	if c == nil {
		return ErrConnectionsUnavailable
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrConnectionsClosed
	}
	var old *sdkmcp.ClientSession
	if index := slices.IndexFunc(c.servers, func(ms *server) bool { return ms.name() == name }); index >= 0 {
		target := c.servers[index]
		old = target.session
		if target.cancel != nil {
			target.cancel()
			target.cancel = nil
		}
		target.generation++
		// slices.Delete clears the vacated pointer, so the long-lived backing
		// array cannot retain the removed session and its verified tool wrappers.
		c.servers = slices.Delete(c.servers, index, index+1)
	}
	c.mu.Unlock()

	// Shrink the model-facing catalog before a potentially-blocking session
	// close. The publication lock keeps this ordered with every dial.
	c.publishTools()
	c.retireSession(old)
	return nil
}

func (c *Connections) retireSession(session *sdkmcp.ClientSession) {
	if session == nil {
		return
	}
	c.mu.Lock()
	owned := c.sessions[session]
	if owned != nil && owned.close == nil {
		attempt := &sessionCloseAttempt{done: make(chan struct{})}
		owned.close = attempt
		go c.closeSessionAttempt(session, owned, attempt)
	}
	c.mu.Unlock()
}

// publishTools rebuilds the model-facing catalog from each connected server's
// last verified tool snapshot. Network I/O happens only while establishing that
// server's session; publication itself is deterministic and cannot turn caller
// cancellation or another server's independent failure into a false catalog.
func (c *Connections) publishTools() {
	c.publishMu.Lock()
	defer c.publishMu.Unlock()

	c.mu.Lock()
	var catalog []tools.Tool
	for _, ms := range c.servers {
		if ms.session != nil {
			catalog = append(catalog, ms.tools...)
		}
	}
	sink := c.onTools
	c.mu.Unlock()

	if sink != nil {
		sink(catalog)
	}
}

// Shutdown rejects new operations, joins all admitted dials, and closes every
// session still present in the ownership ledger. A failed close remains in the
// ledger for a later explicit Shutdown call.
func (c *Connections) Shutdown(ctx context.Context) error {
	if c == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	c.mu.Lock()
	if !c.closed {
		c.closed = true
		for _, ms := range c.servers {
			if ms.cancel != nil {
				ms.cancel()
				ms.cancel = nil
			}
		}
		c.servers = nil
	}
	attempt := c.shutdown
	if attempt != nil {
		select {
		case <-attempt.done:
			if attempt.err == nil {
				c.mu.Unlock()
				return nil
			}
			attempt = nil
		default:
		}
	}
	if attempt == nil {
		attempt = &shutdownAttempt{done: make(chan struct{})}
		c.shutdown = attempt
		go c.closeAll(attempt)
	}
	c.mu.Unlock()

	select {
	case <-attempt.done:
		return attempt.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Connections) closeAll(attempt *shutdownAttempt) {
	c.attempts.Wait()

	c.mu.Lock()
	sessions := make([]*sdkmcp.ClientSession, 0, len(c.sessions))
	for session := range c.sessions {
		sessions = append(sessions, session)
	}
	c.mu.Unlock()

	var errs []error
	for _, session := range sessions {
		if err := c.closeSession(context.Background(), session); err != nil {
			errs = append(errs, err)
		}
	}

	c.mu.Lock()
	attempt.err = errors.Join(errs...)
	close(attempt.done)
	c.mu.Unlock()
}

func (c *Connections) closeSession(ctx context.Context, session *sdkmcp.ClientSession) error {
	if session == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	c.mu.Lock()
	owned := c.sessions[session]
	if owned == nil {
		c.mu.Unlock()
		return nil
	}
	attempt := owned.close
	if attempt == nil {
		attempt = &sessionCloseAttempt{done: make(chan struct{})}
		owned.close = attempt
		go c.closeSessionAttempt(session, owned, attempt)
	}
	c.mu.Unlock()

	select {
	case <-attempt.done:
		return attempt.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Connections) closeSessionAttempt(
	session *sdkmcp.ClientSession,
	owned *ownedSession,
	attempt *sessionCloseAttempt,
) {
	err := owned.closeFn()
	c.mu.Lock()
	if current := c.sessions[session]; current == owned && current.close == attempt {
		if err == nil {
			delete(c.sessions, session)
		} else {
			// The failed attempt remains joinable through its local pointer, but
			// ownership is ready for one later explicit close generation.
			current.close = nil
		}
	}
	attempt.err = err
	close(attempt.done)
	c.mu.Unlock()
}
