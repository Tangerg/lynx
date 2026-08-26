package mcp

import (
	"context"
	"errors"
	"fmt"
	"slices"

	toolcontract "github.com/Tangerg/lynx/core/tool"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

// toolListTarget is a connected server/session pair snapshotted under the lock
// so live tools/list RPCs can run outside it.
type toolListTarget struct {
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
	for _, configuredServer := range c.servers {
		out = append(out, mcpserver.ConnectionStatus{
			Name:      configuredServer.name(),
			State:     configuredServer.state,
			ToolCount: len(configuredServer.tools),
		})
	}
	return out
}

// Tools lists the tools advertised by the connected servers, scoped to server
// when non-empty. It queries each session's tools/list live, ordered by
// (server, tool name) as dialed. Nil-safe.
func (c *Connections) Tools(ctx context.Context, serverName string) ([]mcpserver.AdvertisedTool, error) {
	if c == nil {
		return nil, nil
	}
	// Snapshot the connected (name, session) pairs under the lock, then run the
	// live tools/list RPCs outside it — a slow upstream mustn't block reconnect
	// or status reads. A session closed by a racing reconnect just errors here.
	c.mu.Lock()
	var targets []toolListTarget
	for _, configuredServer := range c.servers {
		if configuredServer.session == nil || (serverName != "" && configuredServer.name() != serverName) {
			continue
		}
		targets = append(targets, toolListTarget{configuredServer.name(), configuredServer.session})
	}
	c.mu.Unlock()

	var out []mcpserver.AdvertisedTool
	for _, t := range targets {
		seen := make(map[string]struct{})
		for descriptor, err := range t.session.Tools(ctx, nil) {
			if err != nil {
				return nil, fmt.Errorf("mcp: list tools from server %q: %w", t.name, err)
			}
			if descriptor == nil || descriptor.Name == "" {
				return nil, fmt.Errorf("%w: server %q returned a nil or unnamed tool", mcpserver.ErrInvalidRemoteToolCatalog, t.name)
			}
			if _, duplicate := seen[descriptor.Name]; duplicate {
				return nil, fmt.Errorf("%w: server %q returned duplicate tool %q", mcpserver.ErrInvalidRemoteToolCatalog, t.name, descriptor.Name)
			}
			if err := mcpserver.ValidateRemoteToolCount(len(seen) + 1); err != nil {
				return nil, fmt.Errorf("mcp: validate tools from server %q: %w", t.name, err)
			}
			if err := mcpserver.ValidateRemoteToolDescription(descriptor.Description); err != nil {
				return nil, fmt.Errorf("mcp: validate tool %q from server %q: %w", descriptor.Name, t.name, err)
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
			seen[descriptor.Name] = struct{}{}
			out = append(out, mcpserver.AdvertisedTool{
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
	var detachedSession *sdkmcp.ClientSession
	if index := slices.IndexFunc(c.servers, func(configuredServer *server) bool { return configuredServer.name() == name }); index >= 0 {
		target := c.servers[index]
		detachedSession = target.session
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
	c.retireSession(detachedSession)
	return nil
}

func (c *Connections) retireSession(session *sdkmcp.ClientSession) {
	if session == nil {
		return
	}
	c.mu.Lock()
	attempt := c.beginSessionCloseLocked(session)
	if attempt != nil {
		if c.retirements == nil {
			c.retirements = make(map[*sessionCloseAttempt]struct{})
		}
		c.retirements[attempt] = struct{}{}
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
	var catalog []toolcontract.Tool
	for _, configuredServer := range c.servers {
		if configuredServer.session != nil {
			catalog = append(catalog, configuredServer.tools...)
		}
	}
	sink := c.onTools
	c.mu.Unlock()

	if sink != nil {
		sink(catalog)
	}
}

// Shutdown rejects new operations, joins all admitted dials, and closes every
// session still present in the ownership ledger. ClientSession.Close consumes
// its transport closer even when it returns an error, so that diagnostic is
// terminal and the session leaves the ledger after the attempt completes.
func (c *Connections) Shutdown(ctx context.Context) error {
	if c == nil {
		return nil
	}
	if ctx == nil {
		return errors.New("mcp: shutdown context is required")
	}
	c.mu.Lock()
	if !c.closed {
		c.closed = true
		for _, configuredServer := range c.servers {
			if configuredServer.cancel != nil {
				configuredServer.cancel()
				configuredServer.cancel = nil
			}
		}
		c.servers = nil
	}
	attempt := c.shutdown
	if attempt != nil {
		select {
		case <-attempt.done:
			// Every session close owned by the completed generation reached its
			// terminal state. Its diagnostic was reported to callers that joined
			// that generation; repeating Shutdown is an idempotent no-op.
			c.mu.Unlock()
			return nil
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
	closeAttempts := make([]*sessionCloseAttempt, 0, len(c.sessions)+len(c.retirements))
	seen := make(map[*sessionCloseAttempt]struct{}, len(c.sessions)+len(c.retirements))
	for session := range c.sessions {
		closeAttempt := c.beginSessionCloseLocked(session)
		if closeAttempt != nil {
			if _, ok := seen[closeAttempt]; !ok {
				seen[closeAttempt] = struct{}{}
				closeAttempts = append(closeAttempts, closeAttempt)
			}
		}
	}
	for closeAttempt := range c.retirements {
		if _, ok := seen[closeAttempt]; !ok {
			seen[closeAttempt] = struct{}{}
			closeAttempts = append(closeAttempts, closeAttempt)
		}
	}
	c.mu.Unlock()

	var errs []error
	for _, closeAttempt := range closeAttempts {
		<-closeAttempt.done
		if closeAttempt.err != nil {
			errs = append(errs, closeAttempt.err)
		}
	}

	c.mu.Lock()
	// A racing Detach may register an attempt after the initial snapshot, but
	// the session itself guaranteed that attempt was already in closeAttempts.
	// Consume the diagnostic only after the joined attempt has completed.
	for closeAttempt := range seen {
		delete(c.retirements, closeAttempt)
	}
	attempt.err = errors.Join(errs...)
	close(attempt.done)
	c.mu.Unlock()
}

func (c *Connections) closeSession(ctx context.Context, session *sdkmcp.ClientSession) error {
	if session == nil {
		return nil
	}
	if ctx == nil {
		return errors.New("mcp: session close context is required")
	}
	attempt := c.beginSessionClose(session)
	if attempt == nil {
		return nil
	}

	select {
	case <-attempt.done:
		return attempt.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Connections) beginSessionClose(session *sdkmcp.ClientSession) *sessionCloseAttempt {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.beginSessionCloseLocked(session)
}

func (c *Connections) beginSessionCloseLocked(session *sdkmcp.ClientSession) *sessionCloseAttempt {
	owned := c.sessions[session]
	if owned == nil {
		return nil
	}
	attempt := owned.close
	if attempt == nil {
		attempt = &sessionCloseAttempt{done: make(chan struct{})}
		owned.close = attempt
		go c.closeSessionAttempt(session, owned, attempt)
	}
	return attempt
}

func (c *Connections) closeSessionAttempt(
	session *sdkmcp.ClientSession,
	owned *ownedSession,
	attempt *sessionCloseAttempt,
) {
	err := owned.closeFn()
	c.mu.Lock()
	if current := c.sessions[session]; current == owned && current.close == attempt {
		// sdkmcp.ClientSession.Close is one-shot: its underlying transport
		// closer is consumed before the error returns. Retaining this entry
		// would only replay a cached diagnostic, never advance cleanup.
		delete(c.sessions, session)
	}
	if err == nil {
		// Successful asynchronous retirement has no diagnostic to preserve for
		// Shutdown; a failed attempt stays in retirements until it is reported.
		delete(c.retirements, attempt)
	}
	attempt.err = err
	close(attempt.done)
	c.mu.Unlock()
}
