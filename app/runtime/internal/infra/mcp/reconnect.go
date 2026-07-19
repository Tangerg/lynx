package mcp

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"go.opentelemetry.io/otel/trace"

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
	// Serialize reconnects: without this, two concurrent calls for the same
	// server both dial and the loser's session is overwritten + leaked.
	c.reconnectMu.Lock()
	defer c.reconnectMu.Unlock()

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
	c.mu.Unlock()

	// Publish the connecting state before closing/dialing: no new turn may keep
	// resolving wrappers backed by the session we are about to close.
	c.publishTools()

	// Close the old session outside the lock — Close may block on I/O.
	if old != nil {
		recordCleanupError(ctx, old.Close())
	}

	return c.dialAndSwap(ctx, ms, cfg, false)
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
	c.reconnectMu.Lock()
	defer c.reconnectMu.Unlock()

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
	c.mu.Unlock()

	c.publishTools()

	if old != nil {
		recordCleanupError(ctx, old.Close())
	}

	return c.dialAndSwap(ctx, ms, cfg, false)
}

func reusableOAuth(current, candidate ServerConfig, handler auth.OAuthHandler) auth.OAuthHandler {
	if handler == nil ||
		current.Transport != TransportHTTP ||
		candidate.Transport != TransportHTTP ||
		!mcpserver.SameHTTPOrigin(current.Endpoint, candidate.Endpoint) {
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
	c.reconnectMu.Lock()
	defer c.reconnectMu.Unlock()

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
	c.mu.Unlock()

	c.publishTools()

	if old != nil {
		recordCleanupError(ctx, old.Close())
	}

	// Bound the human-in-the-loop flow here; clear the per-server handshake
	// timeout so it can't abort the browser wait mid-sign-in.
	ctx, cancel := context.WithTimeout(ctx, oauthFlowTimeout)
	defer cancel()
	cfg.Timeout = 0

	flow, err := newOAuthFlow()
	if err != nil {
		c.setState(ms, mcpserver.ConnectionFailed, err)
		return err
	}
	defer flow.close(ctx)
	handler, err := newOAuthHandler(flow)
	if err != nil {
		c.setState(ms, mcpserver.ConnectionFailed, err)
		return err
	}
	cfg.OAuthHandler = handler

	return c.dialAndSwap(ctx, ms, cfg, true)
}

// dialAndSwap dials cfg, proves the session with a tools/list, then publishes it
// on ms under the lock — the shared tail of [Connections.Reconnect] /
// [Connections.Configure] / [Connections.Authorize]. Call with reconnectMu held
// and c.mu NOT held (Dial blocks on I/O outside the lock). keepHandler stores
// cfg.OAuthHandler on ms after a successful connect (Authorize keeps the
// just-authorized handler for this session's later reconnects; the plain dials
// reuse an existing one and pass false).
func (c *Connections) dialAndSwap(ctx context.Context, ms *server, cfg ServerConfig, keepHandler bool) error {
	session, err := dial(ctx, c.client, cfg)
	var verifiedTools []tools.Tool
	if err == nil {
		// Prove the session is usable before publishing it as connected.
		verifiedTools, err = sourceTools(ctx, lynxmcp.ToolSource{Name: cfg.Name, Session: session})
	}
	if err == nil {
		c.mu.Lock()
		err = validateToolCatalog(c.servers, ms, cfg.Name, verifiedTools)
		c.mu.Unlock()
	}
	if err != nil && session != nil {
		err = errors.Join(err, session.Close())
		session = nil
	}

	c.mu.Lock()
	if c.closed {
		// Close ran while we were dialing outside the lock: it niled c.servers
		// (so this ms is detached) and closed every session. Storing the fresh
		// session here would strand it past Close's sweep — a connection leak.
		// Drop it instead. Mirrors lsp.Servers.clientFor's closed re-check.
		c.mu.Unlock()
		var closeErr error
		if session != nil {
			closeErr = session.Close()
		}
		return errors.Join(ErrConnectionsClosed, err, closeErr)
	}
	if err != nil {
		ms.session, ms.tools, ms.state, ms.lastErr = nil, nil, dialStatus(err), err
	} else {
		ms.session, ms.tools, ms.state, ms.lastErr = session, verifiedTools, mcpserver.ConnectionConnected, nil
		if keepHandler {
			ms.oauth = cfg.OAuthHandler // keep the authorized handler for this session's reconnects
		}
	}
	c.mu.Unlock()

	// Publish only the snapshots proved above. Re-querying every other server
	// here would let an unrelated transient tools/list failure or cancellation
	// silently erase its tools while its status remained connected.
	c.publishTools()
	return err
}

func recordCleanupError(ctx context.Context, err error) {
	if err != nil {
		trace.SpanFromContext(ctx).RecordError(err)
	}
}

// setState records a terminal dial outcome on one server under the lock — the
// shared tail for the early-failure paths that don't reach the dial.
func (c *Connections) setState(ms *server, state mcpserver.ConnectionState, err error) {
	c.mu.Lock()
	ms.session, ms.tools, ms.state, ms.lastErr = nil, nil, state, err
	c.mu.Unlock()
}
