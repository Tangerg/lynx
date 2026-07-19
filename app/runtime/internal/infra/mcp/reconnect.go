package mcp

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/app/runtime/internal/component/httporigin"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	lynxmcp "github.com/Tangerg/lynx/mcp"
	"github.com/Tangerg/lynx/tools"
)

// Reconnect tears down a configured server's current session (if any) and
// re-dials it, then rebuilds the live model-facing tool set and pushes it to
// the tool sink so the model immediately sees the refreshed server. The status
// walks connecting -> (connected | failed). Returns [ErrUnknownServer] for an
// unconfigured name.
func (c *Connections) Reconnect(ctx context.Context, name string) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrConnectionsClosed
	}
	ms := c.find(name)
	if ms == nil {
		c.mu.Unlock()
		return fmt.Errorf("%w: %q", ErrUnknownServer, name)
	}
	old := ms.session
	ms.session = nil
	ms.tools = nil
	ms.state = mcpserver.ConnectionConnecting
	ms.lastErr = nil
	cfg := ms.config
	cfg.OAuthHandler = ms.oauth // reuse this session's sign-in (nil for non-OAuth)
	attempt := c.beginAttempt(ctx, ms)
	c.mu.Unlock()
	defer c.finishAttempt(attempt)

	// Publish the connecting state before closing/dialing: no new turn may keep
	// resolving wrappers backed by the session we are about to close.
	c.publishTools()

	// Close the old session outside the lock — Close may block on I/O.
	if old != nil {
		recordCleanupError(ctx, old.Close())
	}

	return c.dialAndSwap(attempt, cfg, false)
}

// Configure adds a new server or re-dials an existing one with the given
// config, then refreshes the model-facing tool set so the model immediately
// sees the (re)connected server. It is the runtime-mutable counterpart to the
// boot-time [Dial]: mcp.configs.configure / enabling a server routes here.
// Serialized with [Reconnect] (both dial + swap a session). Nil-safe only on a
// nil receiver is NOT supported — Configure mutates and a nil here is a wiring
// bug, so callers hold a real *Connections.
func (c *Connections) Configure(ctx context.Context, cfg ServerConfig) error {
	cfg = cloneServerConfig(cfg)
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("mcp: invalid server %q: %w", cfg.Name, err)
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrConnectionsClosed
	}
	ms := c.find(cfg.Name)
	if ms == nil {
		ms = &server{config: cfg}
		c.servers = append(c.servers, ms)
	}
	oauth := reusableOAuth(ms.config, cfg, ms.oauth)
	ms.oauth = oauth
	old := ms.session
	ms.config = cfg
	ms.session = nil
	ms.tools = nil
	ms.state = mcpserver.ConnectionConnecting
	ms.lastErr = nil
	cfg.OAuthHandler = oauth // only reusable while the configured origin is unchanged
	attempt := c.beginAttempt(ctx, ms)
	c.mu.Unlock()
	defer c.finishAttempt(attempt)

	c.publishTools()

	if old != nil {
		recordCleanupError(ctx, old.Close())
	}

	return c.dialAndSwap(attempt, cfg, false)
}

func reusableOAuth(current, candidate ServerConfig, handler auth.OAuthHandler) auth.OAuthHandler {
	if handler == nil ||
		current.Transport != TransportHTTP ||
		candidate.Transport != TransportHTTP ||
		!httporigin.Same(current.Endpoint, candidate.Endpoint) {
		return nil
	}
	return handler
}

// Authorize runs the interactive OAuth sign-in for an HTTP server: it opens the
// system browser to the authorization URL, catches the redirect on a loopback
// callback, and (via the go-sdk) discovers + dynamically registers + exchanges
// the code. On success the live OAuth handler is kept on the server (reused by
// later reconnects this session, auto-refreshing) and the server connects. The
// handler is NOT persisted — a restart re-prompts. Blocks until the user
// completes the browser flow or [oauthFlowTimeout] elapses. Returns
// [ErrUnknownServer] for an unconfigured name. Serialized with the other dials.
func (c *Connections) Authorize(ctx context.Context, name string) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrConnectionsClosed
	}
	ms := c.find(name)
	if ms == nil {
		c.mu.Unlock()
		return fmt.Errorf("%w: %q", ErrUnknownServer, name)
	}
	if ms.config.Transport != TransportHTTP {
		c.mu.Unlock()
		return errors.New("mcp: OAuth applies to HTTP servers only")
	}
	old := ms.session
	ms.session = nil
	ms.tools = nil
	ms.state = mcpserver.ConnectionConnecting
	ms.lastErr = nil
	cfg := ms.config
	attempt := c.beginAttempt(ctx, ms)
	c.mu.Unlock()
	defer c.finishAttempt(attempt)

	c.publishTools()

	if old != nil {
		recordCleanupError(ctx, old.Close())
	}

	// Bound the human-in-the-loop flow here; clear the per-server handshake
	// timeout so it can't abort the browser wait mid-sign-in.
	ctx, cancel := context.WithTimeout(attempt.ctx, oauthFlowTimeout)
	defer cancel()
	cfg.Timeout = 0

	flow, err := newOAuthFlow()
	if err != nil {
		c.failAttempt(attempt, err)
		return err
	}
	defer flow.close(ctx)
	handler, err := newOAuthHandler(flow)
	if err != nil {
		c.failAttempt(attempt, err)
		return err
	}
	cfg.OAuthHandler = handler

	attempt.ctx = ctx
	return c.dialAndSwap(attempt, cfg, true)
}

// dialAndSwap dials cfg, proves the session with a tools/list, then publishes it
// on ms under the lock — the shared tail of [Connections.Reconnect] /
// [Connections.Configure] / [Connections.Authorize]. The per-server generation
// rejects a stale completion after a newer configure/remove/reconnect. c.mu is
// not held while dialing. keepHandler stores
// cfg.OAuthHandler on ms after a successful connect (Authorize keeps the
// just-authorized handler for this session's later reconnects; the plain dials
// reuse an existing one and pass false).
func (c *Connections) dialAndSwap(attempt connectionAttempt, cfg ServerConfig, keepHandler bool) error {
	session, err := dial(attempt.ctx, c.client, cfg)
	var verifiedTools []tools.Tool
	if err == nil {
		// Prove the session is usable before publishing it as connected.
		verifiedTools, err = sourceTools(attempt.ctx, lynxmcp.ToolSource{Name: cfg.Name, Session: session})
	}
	if err == nil {
		c.mu.Lock()
		err = validateToolCatalog(c.servers, attempt.target, cfg.Name, verifiedTools)
		c.mu.Unlock()
	}
	if err != nil && session != nil {
		err = errors.Join(err, session.Close())
		session = nil
	}

	c.mu.Lock()
	current := c.currentAttempt(attempt)
	if c.closed || !current {
		// Close ran while we were dialing outside the lock: it niled c.servers
		// (so this ms is detached) and closed every session. Storing the fresh
		// session here would strand it past Close's sweep — a connection leak.
		// Drop it instead. Mirrors lsp.Servers.clientFor's closed re-check.
		c.mu.Unlock()
		var closeErr error
		if session != nil {
			closeErr = session.Close()
		}
		if c.closed {
			return errors.Join(ErrConnectionsClosed, err, closeErr)
		}
		return errors.Join(errConnectionSuperseded, err, closeErr)
	}
	if err != nil {
		attempt.target.session, attempt.target.tools, attempt.target.state, attempt.target.lastErr = nil, nil, dialStatus(err), err
	} else {
		attempt.target.session, attempt.target.tools, attempt.target.state, attempt.target.lastErr = session, verifiedTools, mcpserver.ConnectionConnected, nil
		if keepHandler {
			attempt.target.oauth = cfg.OAuthHandler // keep the authorized handler for this session's reconnects
		}
	}
	attempt.target.cancel = nil
	c.mu.Unlock()

	// Publish only the snapshots proved above. Re-querying every other server
	// here would let an unrelated transient tools/list failure or cancellation
	// silently erase its tools while its status remained connected.
	c.publishTools()
	return err
}

var errConnectionSuperseded = errors.New("mcp: connection operation superseded")

type connectionAttempt struct {
	target     *server
	generation uint64
	ctx        context.Context
	cancel     context.CancelFunc
}

// beginAttempt is called with c.mu held. It cancels only the previous operation
// for this server; unrelated servers continue independently.
func (c *Connections) beginAttempt(parent context.Context, target *server) connectionAttempt {
	if target.cancel != nil {
		target.cancel()
	}
	target.generation++
	ctx, cancel := context.WithCancel(parent)
	target.cancel = cancel
	c.attempts.Add(1)
	return connectionAttempt{target: target, generation: target.generation, ctx: ctx, cancel: cancel}
}

func (c *Connections) finishAttempt(attempt connectionAttempt) {
	attempt.cancel()
	c.attempts.Done()
}

// currentAttempt is called with c.mu held.
func (c *Connections) currentAttempt(attempt connectionAttempt) bool {
	return c.find(attempt.target.name()) == attempt.target &&
		attempt.target.generation == attempt.generation
}

func (c *Connections) failAttempt(attempt connectionAttempt, err error) {
	c.mu.Lock()
	if c.currentAttempt(attempt) {
		attempt.target.session = nil
		attempt.target.tools = nil
		attempt.target.state = mcpserver.ConnectionFailed
		attempt.target.lastErr = err
		attempt.target.cancel = nil
	}
	c.mu.Unlock()
}

func recordCleanupError(ctx context.Context, err error) {
	if err != nil {
		trace.SpanFromContext(ctx).RecordError(err)
	}
}

// setState records a terminal dial outcome on one server under the lock — the
// shared tail for the early-failure paths that don't reach the dial.
