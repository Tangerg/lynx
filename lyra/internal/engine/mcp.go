package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/mcp"
)

// McpToolInfo is one tool advertised by a connected MCP server — the
// client-facing projection (workspace.mcp.listTools) of a remote tool
// descriptor. Name is the bare (un-prefixed) tool name; Server is the source
// it belongs to. Tools reach the model under "<server>_<name>", but the wire
// view keeps the two fields separate.
type McpToolInfo struct {
	Server      string
	Name        string
	Description string
	InputSchema map[string]any
}

// MCP server status (wire vocabulary, AUX_API §5.1). "connecting" is the
// transient state during a reconnect; "needsAuth" stays unproduced until the
// dial surfaces an auth-distinguishable error — not faked until then.
const (
	mcpConnected  = "connected"
	mcpConnecting = "connecting"
	mcpFailed     = "failed"
)

// mcpServer is the live state of one configured MCP server: its config (kept
// so reconnect can re-dial by name), its session (nil when not connected), its
// status, and the failure reason. Boot tolerates a per-server failure — it
// records "failed" and keeps going, so one unreachable server no longer fails
// engine startup and the client sees the rest plus the broken one's reason
// (workspace.mcp.listServers).
//
// Mutated only by [Engine.ReconnectMCPServer] after boot; all field access is
// guarded by Engine.mcpMu (boot writes happen-before the engine is returned).
type mcpServer struct {
	config  mcp.ServerConfig
	session *sdkmcp.ClientSession // nil when not connected
	status  string
	lastErr error
}

// name is the server's configured name (its stable identity).
func (m *mcpServer) name() string { return m.config.Name }

// McpServerStatus is the per-server connection state the runtime exposes to
// workspace.mcp.listServers (AUX_API §5.1). Status uses the wire vocabulary;
// Err is the dial / tools-list failure reason, set only when Status is
// "failed" (the server layer renders it as McpServer.Error).
type McpServerStatus struct {
	Name   string
	Status string
	Err    error
}

// MCPServerStatuses returns one entry per CONFIGURED MCP server (connected and
// failed alike), in dial order — the truthful server list workspace.mcp.
// listServers renders, replacing the old "everything is connected" assumption
// that only held because a dial failure used to fail startup.
func (e *Engine) MCPServerStatuses() []McpServerStatus {
	e.mcpMu.Lock()
	defer e.mcpMu.Unlock()
	out := make([]McpServerStatus, 0, len(e.mcpServers))
	for _, ms := range e.mcpServers {
		out = append(out, McpServerStatus{Name: ms.name(), Status: ms.status, Err: ms.lastErr})
	}
	return out
}

// MCPTools lists the tools advertised by the connected MCP servers, scoped to
// server when non-empty (empty = every connected server). It queries each
// session's tools/list live (not the model-facing flat tool set), so the
// per-server breakdown stays accurate without parsing name prefixes. A failed
// (unconnected) or unknown server yields an empty list. Ordered by (server,
// tool name) as the sources were dialed.
func (e *Engine) MCPTools(ctx context.Context, server string) ([]McpToolInfo, error) {
	// Snapshot the connected (name, session) pairs under the lock, then run the
	// live tools/list RPCs outside it — a slow upstream mustn't block reconnect
	// or status reads. A session closed by a racing reconnect just errors here.
	type target struct {
		name    string
		session *sdkmcp.ClientSession
	}
	e.mcpMu.Lock()
	var targets []target
	for _, ms := range e.mcpServers {
		if ms.session == nil || (server != "" && ms.name() != server) {
			continue
		}
		targets = append(targets, target{ms.name(), ms.session})
	}
	e.mcpMu.Unlock()

	var out []McpToolInfo
	for _, t := range targets {
		for descriptor, err := range t.session.Tools(ctx, nil) {
			if err != nil {
				return nil, fmt.Errorf("engine: list tools from MCP server %q: %w", t.name, err)
			}
			out = append(out, McpToolInfo{
				Server:      t.name,
				Name:        descriptor.Name,
				Description: descriptor.Description,
				InputSchema: schemaToMap(descriptor.InputSchema),
			})
		}
	}
	return out, nil
}

// schemaToMap renders an MCP tool's input schema (an opaque schema value, as
// the SDK types it) as a generic object for the wire. A nil schema or a
// marshal failure yields nil (the wire field is optional) rather than erroring
// a whole listing over one odd schema.
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

// engineTracer emits the lyra-runtime startup / orchestration spans the
// lower layers don't (MCP dial). Per-call MCP tool spans come from the
// mcp module itself. No-op until a TracerProvider is installed.
var engineTracer = otel.Tracer("lynx/lyra/engine")

// dialMCPServers connects to each configured MCP server, lists its tools, and
// returns the merged model-facing tool list alongside the per-server state
// (each carrying its open session, status, and failure reason) so the caller
// can enumerate / close them and report status (workspace.mcp.listServers).
// The server name namespaces tools across servers.
//
// Failure semantics, two tiers:
//   - A config mistake — duplicate name or an invalid entry ([mcp.ServerConfig.
//     Validate], e.g. empty endpoint / command) — is FATAL: validated up front,
//     before any dial, so the operator sees the typo at boot and nothing leaks.
//   - A reachability failure — a well-formed server that can't be reached, or
//     whose first tools/list fails — is TOLERATED: recorded "failed" (with its
//     reason) and skipped, so boot continues and the bad server degrades to
//     "missing tools + a visible failure" rather than a dead runtime.
func dialMCPServers(ctx context.Context, client *sdkmcp.Client, servers []mcp.ServerConfig) (booted []*mcpServer, tools []chat.Tool, err error) {
	if len(servers) == 0 {
		return nil, nil, nil
	}

	// Validate config before dialing anything: duplicate names collide tool
	// prefixes, and a malformed entry can never work — both are operator
	// mistakes that should fail loudly at boot, not degrade to a "failed" row.
	seen := make(map[string]struct{}, len(servers))
	for _, srv := range servers {
		if _, dup := seen[srv.Name]; dup {
			return nil, nil, fmt.Errorf("engine: duplicate MCP server name %q", srv.Name)
		}
		seen[srv.Name] = struct{}{}
		if verr := srv.Validate(); verr != nil {
			return nil, nil, fmt.Errorf("engine: invalid MCP server %q: %w", srv.Name, verr)
		}
	}

	ctx, span := engineTracer.Start(ctx, "mcp.dial_servers",
		trace.WithAttributes(attribute.Int("mcp.server.count", len(servers))))
	defer span.End()

	booted = make([]*mcpServer, 0, len(servers))
	failures := 0
	for _, srv := range servers {
		ms := &mcpServer{config: srv}

		session, derr := mcp.Dial(ctx, client, srv)
		if derr != nil {
			ms.status, ms.lastErr = mcpFailed, derr
			failures++
			booted = append(booted, ms)
			continue
		}
		srcTools, terr := sourceTools(ctx, mcp.Source{Name: srv.Name, Session: session})
		if terr != nil {
			_ = session.Close() // half-open: drop it rather than keep a session whose tools we can't read
			ms.status, ms.lastErr = mcpFailed, terr
			failures++
			booted = append(booted, ms)
			continue
		}
		ms.session, ms.status = session, mcpConnected
		tools = append(tools, srcTools...)
		booted = append(booted, ms)
	}

	span.SetAttributes(
		attribute.Int("mcp.tool.count", len(tools)),
		attribute.Int("mcp.server.failed", failures),
	)
	if failures > 0 {
		// A degraded boot isn't an error (the runtime still serves), but mark
		// the span so an operator can correlate missing tools to the boot trace.
		span.SetStatus(codes.Error, fmt.Sprintf("%d/%d MCP servers failed to connect", failures, len(servers)))
	}
	return booted, tools, nil
}

// sourceTools lists one MCP source's model-facing tools (prefixed
// "<server>_<tool>" via the provider's default naming). Isolated per source so
// a single server's tools/list failure stays its own — see dialMCPServers.
func sourceTools(ctx context.Context, src mcp.Source) ([]chat.Tool, error) {
	provider, err := mcp.NewProvider(mcp.ProviderConfig{Sources: []mcp.Source{src}})
	if err != nil {
		return nil, fmt.Errorf("engine: build MCP provider for %q: %w", src.Name, err)
	}
	return provider.Tools(ctx)
}

// ErrUnknownMCPServer is returned by [Engine.ReconnectMCPServer] for a name
// that was never configured — the caller (workspace.mcp.reconnect) maps it to
// invalid_params.
var ErrUnknownMCPServer = errors.New("engine: unknown MCP server")

// ReconnectMCPServer tears down a configured server's current session (if any)
// and re-dials it, then rebuilds the live model-facing MCP tool set so the
// model immediately sees the refreshed server. Used by workspace.mcp.reconnect
// to recover a "failed" server (or bounce a flaky "connected" one) without
// restarting the runtime.
//
// The status walks connecting → (connected | failed); the connecting state is
// observable via [Engine.MCPServerStatuses] for the duration of the dial. On a
// dial / tools-list failure the server is left "failed" with its reason and the
// error is returned; the tool set is rebuilt either way (a now-gone server's
// tools drop out, a recovered server's reappear). Returns [ErrUnknownMCPServer]
// for an unconfigured name.
func (e *Engine) ReconnectMCPServer(ctx context.Context, name string) error {
	e.mcpMu.Lock()
	ms := e.findMCPServer(name)
	if ms == nil {
		e.mcpMu.Unlock()
		return fmt.Errorf("%w: %q", ErrUnknownMCPServer, name)
	}
	old := ms.session
	ms.session = nil
	ms.status = mcpConnecting
	ms.lastErr = nil
	cfg := ms.config
	e.mcpMu.Unlock()

	// Close the old session outside the lock — Close may block on I/O.
	if old != nil {
		_ = old.Close()
	}

	session, err := mcp.Dial(ctx, e.mcpClient, cfg)
	if err == nil {
		// Prove the session is usable before publishing it as connected.
		if _, terr := sourceTools(ctx, mcp.Source{Name: name, Session: session}); terr != nil {
			_ = session.Close()
			err = terr
			session = nil
		}
	}

	e.mcpMu.Lock()
	if err != nil {
		ms.session, ms.status, ms.lastErr = nil, mcpFailed, err
	} else {
		ms.session, ms.status, ms.lastErr = session, mcpConnected, nil
	}
	e.mcpMu.Unlock()

	// Rebuild the live tool set from whatever is connected now (this server
	// included or excluded). Outside the lock — it runs tools/list RPCs.
	e.refreshMCPTools(ctx)
	return err
}

// findMCPServer returns the server with the given name, or nil. Caller holds mcpMu.
func (e *Engine) findMCPServer(name string) *mcpServer {
	for _, ms := range e.mcpServers {
		if ms.name() == name {
			return ms
		}
	}
	return nil
}

// refreshMCPTools rebuilds the model-facing MCP tool set from the currently-
// connected sessions and atomically swaps it into the tool resolver, so the
// next turn's resolution sees the post-reconnect set. A per-server tools/list
// error drops just that server's tools (it was already marked failed). Runs the
// RPCs outside mcpMu by snapshotting the connected sessions first.
func (e *Engine) refreshMCPTools(ctx context.Context) {
	type target struct {
		name    string
		session *sdkmcp.ClientSession
	}
	e.mcpMu.Lock()
	var targets []target
	for _, ms := range e.mcpServers {
		if ms.session != nil {
			targets = append(targets, target{ms.name(), ms.session})
		}
	}
	e.mcpMu.Unlock()

	var tools []chat.Tool
	for _, t := range targets {
		srcTools, err := sourceTools(ctx, mcp.Source{Name: t.name, Session: t.session})
		if err != nil {
			continue // already-failed server; skip its tools
		}
		tools = append(tools, srcTools...)
	}
	e.toolResolver.setMCPTools(tools)
}
