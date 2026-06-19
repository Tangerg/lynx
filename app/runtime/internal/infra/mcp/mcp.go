// Package mcp is the MCP-connection infra: it dials configured MCP servers,
// holds their live sessions, lists their tools, and reconnects them — the
// external-system adapter the engine builds its MCP tool set over. Pure infra
// (over the lynx mcp module + the go-sdk client); zero domain knowledge.
//
// A degraded boot is tolerated: a server that can't be reached is recorded
// "failed" and skipped, so one unreachable server never fails startup; only a
// config mistake (duplicate name / invalid entry) is fatal.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/core/model/chat"
	lynxmcp "github.com/Tangerg/lynx/mcp"
)

// ServerConfig is the MCP server descriptor, re-exported so callers configure
// connections without importing the lynx mcp module directly.
type ServerConfig = lynxmcp.ServerConfig

// ErrUnknownServer is returned by [Connections.Reconnect] for a name that was
// never configured — the delivery layer maps it to invalid_params.
var ErrUnknownServer = errors.New("mcp: unknown server")

// Connection status (wire vocabulary, AUX_API §5.1). "connecting" is the
// transient reconnect state; "needsAuth" is produced when a dial fails with an
// auth-distinguishable error (a 401 / Unauthorized), so the client can prompt
// for credentials rather than treat it as a generic "failed".
const (
	statusConnected  = "connected"
	statusConnecting = "connecting"
	statusFailed     = "failed"
	statusNeedsAuth  = "needsAuth"
)

// tracer emits the MCP dial / reconnect spans the lower layers don't (per-call
// MCP tool spans come from the mcp module itself). No-op until a provider is
// installed.
var tracer = otel.Tracer("lynx/lyra/infra/mcp")

// ToolInfo is one tool advertised by a connected server — the client-facing
// projection (workspace.mcp.listTools) of a remote tool descriptor. Name is the
// bare (un-prefixed) tool name; Server is the source. Tools reach the model
// under "<server>_<name>", but the wire view keeps the two fields separate.
type ToolInfo struct {
	Server      string
	Name        string
	Description string
	InputSchema map[string]any
}

// ServerStatus is the per-server connection state exposed to
// workspace.mcp.listServers. Err is the dial / tools-list failure reason, set
// only when Status is "failed".
type ServerStatus struct {
	Name   string
	Status string
	Err    error
}

// server is the live state of one configured MCP server. Mutated only by
// [Connections.Reconnect] after boot; access guarded by Connections.mu.
type server struct {
	config  ServerConfig
	session *sdkmcp.ClientSession // nil when not connected
	status  string
	lastErr error
}

func (s *server) name() string { return s.config.Name }

// Connections owns the live MCP server sessions + reconnect. The optional tool
// sink is invoked with the rebuilt model-facing tool set after a reconnect, so
// the engine can hot-swap the live set into its resolver.
type Connections struct {
	mu      sync.Mutex
	servers []*server
	client  *sdkmcp.Client
	onTools func([]chat.Tool) // tool sink; nil until SetToolSink
	closed  bool              // set by Close; Reconnect checks it after dialing

	// reconnectMu serializes Reconnect so two concurrent calls can't both dial
	// and leak the loser's freshly-dialed session (the winner overwrites
	// ms.session). Separate from mu — held across the dial I/O, which mu (the
	// hot-path registry lock) must not be. Reconnect is a rare admin op, so
	// serializing across servers is fine.
	reconnectMu sync.Mutex
}

// SetToolSink registers the callback Reconnect invokes with the rebuilt
// model-facing MCP tool set (the engine wires it to its resolver's hot-swap).
func (c *Connections) SetToolSink(sink func([]chat.Tool)) { c.onTools = sink }

// Dial connects to each configured server, lists its tools, and returns the
// Connections handle alongside the merged model-facing tool list. The server
// name namespaces tools across servers.
//
// Failure, two tiers: a config mistake (duplicate name / invalid entry) is
// FATAL (validated before any dial); a reachability failure is TOLERATED
// (recorded "failed" and skipped). An empty config yields a nil Connections.
func Dial(ctx context.Context, servers []ServerConfig) (*Connections, []chat.Tool, error) {
	if len(servers) == 0 {
		return &Connections{}, nil, nil
	}

	// Validate config before dialing: duplicate names collide tool prefixes and
	// a malformed entry can never work — operator mistakes that should fail
	// loudly at boot, not degrade to a "failed" row.
	seen := make(map[string]struct{}, len(servers))
	for _, srv := range servers {
		if _, dup := seen[srv.Name]; dup {
			return nil, nil, fmt.Errorf("mcp: duplicate server name %q", srv.Name)
		}
		seen[srv.Name] = struct{}{}
		if verr := srv.Validate(); verr != nil {
			return nil, nil, fmt.Errorf("mcp: invalid server %q: %w", srv.Name, verr)
		}
	}

	// One client identity for every server — none of lyra's connections need
	// per-server handlers (sampling / list-changed), so they share it. Retained
	// so Reconnect can re-dial with it.
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "runtime", Version: "v0.1.0"}, nil)
	c := &Connections{client: client}

	ctx, span := tracer.Start(ctx, "mcp.dial_servers",
		trace.WithAttributes(attribute.Int("mcp.server.count", len(servers))))
	defer span.End()

	var tools []chat.Tool
	failures := 0
	for _, srv := range servers {
		ms := &server{config: srv}
		session, derr := lynxmcp.Dial(ctx, client, srv)
		if derr != nil {
			ms.status, ms.lastErr = dialStatus(derr), derr
			failures++
			c.servers = append(c.servers, ms)
			continue
		}
		srcTools, terr := sourceTools(ctx, lynxmcp.Source{Name: srv.Name, Session: session})
		if terr != nil {
			_ = session.Close() // half-open: drop it rather than keep a session whose tools we can't read
			ms.status, ms.lastErr = statusFailed, terr
			failures++
			c.servers = append(c.servers, ms)
			continue
		}
		ms.session, ms.status = session, statusConnected
		tools = append(tools, srcTools...)
		c.servers = append(c.servers, ms)
	}

	span.SetAttributes(
		attribute.Int("mcp.tool.count", len(tools)),
		attribute.Int("mcp.server.failed", failures),
	)
	if failures > 0 {
		span.SetStatus(codes.Error, fmt.Sprintf("%d/%d MCP servers failed to connect", failures, len(servers)))
	}
	return c, tools, nil
}

// Statuses returns one entry per CONFIGURED server (connected and failed
// alike), in dial order. Nil-safe.
func (c *Connections) Statuses() []ServerStatus {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]ServerStatus, 0, len(c.servers))
	for _, ms := range c.servers {
		out = append(out, ServerStatus{Name: ms.name(), Status: ms.status, Err: ms.lastErr})
	}
	return out
}

// Tools lists the tools advertised by the connected servers, scoped to server
// when non-empty. It queries each session's tools/list live, ordered by
// (server, tool name) as dialed. Nil-safe.
func (c *Connections) Tools(ctx context.Context, server string) ([]ToolInfo, error) {
	if c == nil {
		return nil, nil
	}
	// Snapshot the connected (name, session) pairs under the lock, then run the
	// live tools/list RPCs outside it — a slow upstream mustn't block reconnect
	// or status reads. A session closed by a racing reconnect just errors here.
	type target struct {
		name    string
		session *sdkmcp.ClientSession
	}
	c.mu.Lock()
	var targets []target
	for _, ms := range c.servers {
		if ms.session == nil || (server != "" && ms.name() != server) {
			continue
		}
		targets = append(targets, target{ms.name(), ms.session})
	}
	c.mu.Unlock()

	var out []ToolInfo
	for _, t := range targets {
		for descriptor, err := range t.session.Tools(ctx, nil) {
			if err != nil {
				return nil, fmt.Errorf("mcp: list tools from server %q: %w", t.name, err)
			}
			out = append(out, ToolInfo{
				Server:      t.name,
				Name:        descriptor.Name,
				Description: descriptor.Description,
				InputSchema: schemaToMap(descriptor.InputSchema),
			})
		}
	}
	return out, nil
}

// Reconnect tears down a configured server's current session (if any) and
// re-dials it, then rebuilds the live model-facing tool set and pushes it to
// the tool sink so the model immediately sees the refreshed server. The status
// walks connecting → (connected | failed). Returns [ErrUnknownServer] for an
// unconfigured name.
func (c *Connections) Reconnect(ctx context.Context, name string) error {
	// Serialize reconnects: without this, two concurrent calls for the same
	// server both dial and the loser's session is overwritten + leaked.
	c.reconnectMu.Lock()
	defer c.reconnectMu.Unlock()

	c.mu.Lock()
	ms := c.find(name)
	if ms == nil {
		c.mu.Unlock()
		return fmt.Errorf("%w: %q", ErrUnknownServer, name)
	}
	old := ms.session
	ms.session = nil
	ms.status = statusConnecting
	ms.lastErr = nil
	cfg := ms.config
	c.mu.Unlock()

	// Close the old session outside the lock — Close may block on I/O.
	if old != nil {
		_ = old.Close()
	}

	session, err := lynxmcp.Dial(ctx, c.client, cfg)
	if err == nil {
		// Prove the session is usable before publishing it as connected.
		if _, terr := sourceTools(ctx, lynxmcp.Source{Name: name, Session: session}); terr != nil {
			_ = session.Close()
			err = terr
			session = nil
		}
	}

	c.mu.Lock()
	if c.closed {
		// Close ran while we were dialing outside the lock: it niled c.servers
		// (so this ms is detached) and closed every session. Storing the fresh
		// session here would strand it past Close's sweep — a connection leak.
		// Drop it instead. Mirrors lsp.Manager.clientFor's closed re-check.
		c.mu.Unlock()
		if session != nil {
			_ = session.Close()
		}
		return err
	}
	if err != nil {
		ms.session, ms.status, ms.lastErr = nil, dialStatus(err), err
	} else {
		ms.session, ms.status, ms.lastErr = session, statusConnected, nil
	}
	c.mu.Unlock()

	// Rebuild the live tool set from whatever is connected now and hand it to
	// the sink. Outside the lock — it runs tools/list RPCs.
	c.refreshTools(ctx)
	return err
}

// Configure adds a new server or re-dials an existing one with the given
// config, then refreshes the model-facing tool set so the model immediately
// sees the (re)connected server. It is the runtime-mutable counterpart to the
// boot-time [Dial]: workspace.mcp.configure / enabling a server routes here.
// Serialized with [Reconnect] (both dial + swap a session). Nil-safe only on a
// nil receiver is NOT supported — Configure mutates and a nil here is a wiring
// bug, so callers hold a real *Connections.
func (c *Connections) Configure(ctx context.Context, cfg ServerConfig) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("mcp: invalid server %q: %w", cfg.Name, err)
	}
	c.reconnectMu.Lock()
	defer c.reconnectMu.Unlock()

	c.mu.Lock()
	ms := c.find(cfg.Name)
	if ms == nil {
		ms = &server{config: cfg}
		c.servers = append(c.servers, ms)
	}
	old := ms.session
	ms.config = cfg
	ms.session = nil
	ms.status = statusConnecting
	ms.lastErr = nil
	c.mu.Unlock()

	if old != nil {
		_ = old.Close()
	}

	session, err := lynxmcp.Dial(ctx, c.client, cfg)
	if err == nil {
		if _, terr := sourceTools(ctx, lynxmcp.Source{Name: cfg.Name, Session: session}); terr != nil {
			_ = session.Close()
			err, session = terr, nil
		}
	}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		if session != nil {
			_ = session.Close()
		}
		return err
	}
	if err != nil {
		ms.session, ms.status, ms.lastErr = nil, dialStatus(err), err
	} else {
		ms.session, ms.status, ms.lastErr = session, statusConnected, nil
	}
	c.mu.Unlock()

	c.refreshTools(ctx)
	return err
}

// Remove drops a server from the live set, closing its session, and refreshes
// the model-facing tool set. Removing an unknown name is a no-op (the registry
// is the source of truth; the live set may legitimately lag it). Disabling a
// server routes here too — it stays in the registry but leaves the live set.
func (c *Connections) Remove(ctx context.Context, name string) {
	if c == nil {
		return
	}
	c.reconnectMu.Lock()
	defer c.reconnectMu.Unlock()

	c.mu.Lock()
	var old *sdkmcp.ClientSession
	kept := c.servers[:0]
	for _, ms := range c.servers {
		if ms.name() == name {
			old = ms.session
			continue
		}
		kept = append(kept, ms)
	}
	c.servers = kept
	c.mu.Unlock()

	if old != nil {
		_ = old.Close()
	}
	c.refreshTools(ctx)
}

// Probe dials cfg with a throwaway client, proves its tools are listable, and
// closes the session — a connection test that touches no live state. Used by
// workspace.mcp.test before persisting a configuration. Returns nil on success.
func Probe(ctx context.Context, cfg ServerConfig) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	ctx, span := tracer.Start(ctx, "mcp.probe",
		trace.WithAttributes(attribute.String("mcp.server.name", cfg.Name)))
	defer span.End()

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "runtime-probe", Version: "v0.1.0"}, nil)
	session, err := lynxmcp.Dial(ctx, client, cfg)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	defer session.Close()
	if _, err := sourceTools(ctx, lynxmcp.Source{Name: cfg.Name, Session: session}); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

// find returns the server with the given name, or nil. Caller holds mu.
func (c *Connections) find(name string) *server {
	for _, ms := range c.servers {
		if ms.name() == name {
			return ms
		}
	}
	return nil
}

// refreshTools rebuilds the model-facing tool set from the currently-connected
// sessions and pushes it to the tool sink. A per-server tools/list error drops
// just that server's tools. Runs the RPCs outside mu.
func (c *Connections) refreshTools(ctx context.Context) {
	type target struct {
		name    string
		session *sdkmcp.ClientSession
	}
	c.mu.Lock()
	var targets []target
	for _, ms := range c.servers {
		if ms.session != nil {
			targets = append(targets, target{ms.name(), ms.session})
		}
	}
	c.mu.Unlock()

	var tools []chat.Tool
	for _, t := range targets {
		srcTools, err := sourceTools(ctx, lynxmcp.Source{Name: t.name, Session: t.session})
		if err != nil {
			continue // already-failed server; skip its tools
		}
		tools = append(tools, srcTools...)
	}
	if c.onTools != nil {
		c.onTools(tools)
	}
}

// Close shuts down every open session. Safe to call multiple times. Nil-safe.
func (c *Connections) Close() error {
	if c == nil {
		return nil
	}
	var errs []error
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true // so a Reconnect dialing outside the lock drops its session
	for _, ms := range c.servers {
		if ms.session == nil {
			continue
		}
		if err := ms.session.Close(); err != nil {
			errs = append(errs, err)
		}
		ms.session = nil
	}
	c.servers = nil
	return errors.Join(errs...)
}

// sourceTools lists one MCP source's model-facing tools (prefixed
// "<server>_<tool>" via the provider's default naming). Isolated per source so
// a single server's tools/list failure stays its own.
func sourceTools(ctx context.Context, src lynxmcp.Source) ([]chat.Tool, error) {
	provider, err := lynxmcp.NewProvider(lynxmcp.ProviderConfig{Sources: []lynxmcp.Source{src}})
	if err != nil {
		return nil, fmt.Errorf("mcp: build provider for %q: %w", src.Name, err)
	}
	return provider.Tools(ctx)
}

// dialStatus maps a dial error to the connection status: an
// auth-distinguishable failure becomes "needsAuth" (so the client can prompt
// for credentials), otherwise "failed".
func dialStatus(err error) string {
	if isAuthError(err) {
		return statusNeedsAuth
	}
	return statusFailed
}

// isAuthError reports whether err looks like an MCP server rejecting the
// connection for missing / invalid credentials (HTTP 401 Unauthorized). The
// go-sdk surfaces the transport failure as a wrapped error, so detection is a
// heuristic string match; a false negative just yields the generic "failed"
// status, never a wrong success.
func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "401") || strings.Contains(s, "unauthorized")
}

// schemaToMap renders an MCP tool's input schema as a generic object for the
// wire. A nil schema or a marshal failure yields nil rather than erroring a
// whole listing over one odd schema.
func schemaToMap(schema any) map[string]any {
	if schema == nil {
		return nil
	}
	data, err := json.Marshal(schema)
	if err != nil {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(data, &m) != nil {
		return nil
	}
	return m
}
