package engine

import (
	"context"
	"encoding/json"
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

// MCPTools lists the tools advertised by the connected MCP servers, scoped to
// server when non-empty (empty = every server). It queries each session's
// tools/list live (not the model-facing flat tool set), so the per-server
// breakdown stays accurate without parsing name prefixes. An unknown server
// name yields an empty list (a running runtime only knows servers dialed at
// boot — workspace.mcp.listServers). Ordered by (server, tool name) as the
// sources were dialed.
func (e *Engine) MCPTools(ctx context.Context, server string) ([]McpToolInfo, error) {
	var out []McpToolInfo
	for _, src := range e.mcpSources {
		if server != "" && src.Name != server {
			continue
		}
		for descriptor, err := range src.Session.Tools(ctx, nil) {
			if err != nil {
				return nil, fmt.Errorf("engine: list tools from MCP server %q: %w", src.Name, err)
			}
			out = append(out, McpToolInfo{
				Server:      src.Name,
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

// dialMCPServers connects to each configured MCP server, lists tools,
// and returns the merged tool list alongside the named sources (each
// wrapping an open session) so the caller can close them at shutdown and
// enumerate tools per server (workspace.mcp.listTools). The source name
// also namespaces tools across servers.
//
// Failure semantics: any single server that can't be reached (or is
// misconfigured — [mcp.Dial] validates the config) fails the whole call,
// so the operator sees the problem at startup rather than discovering a
// tool went missing silently. Already-dialed sessions are closed before
// returning the error so we don't leak subprocesses or HTTP connections.
func dialMCPServers(ctx context.Context, servers []mcp.ServerConfig) (tools []chat.Tool, dialed []mcp.Source, err error) {
	if len(servers) == 0 {
		return nil, nil, nil
	}

	ctx, span := engineTracer.Start(ctx, "mcp.dial_servers",
		trace.WithAttributes(attribute.Int("mcp.server.count", len(servers))))
	// Record on whichever error path returns — startup MCP failures are the
	// kind an operator most needs to see correlated to the boot trace.
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()

	// One client identity for every server — none of lyra's MCP
	// connections need per-server client handlers (sampling /
	// list-changed), so they share it.
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "runtime", Version: "v0.1.0"}, nil)

	seenNames := make(map[string]struct{}, len(servers))
	dialed = make([]mcp.Source, 0, len(servers))
	closeAll := func() {
		for _, src := range dialed {
			_ = src.Session.Close()
		}
	}

	for _, srv := range servers {
		if _, dup := seenNames[srv.Name]; dup {
			closeAll()
			return nil, nil, fmt.Errorf("engine: duplicate MCP server name %q", srv.Name)
		}
		seenNames[srv.Name] = struct{}{}

		session, derr := mcp.Dial(ctx, client, srv)
		if derr != nil {
			closeAll()
			return nil, nil, fmt.Errorf("engine: dial MCP server %q: %w", srv.Name, derr)
		}
		dialed = append(dialed, mcp.Source{Name: srv.Name, Session: session})
	}

	provider, perr := mcp.NewProvider(mcp.ProviderConfig{Sources: dialed})
	if perr != nil {
		closeAll()
		return nil, nil, fmt.Errorf("engine: build MCP provider: %w", perr)
	}

	tools, terr := provider.Tools(ctx)
	if terr != nil {
		closeAll()
		return nil, nil, fmt.Errorf("engine: list MCP tools: %w", terr)
	}

	span.SetAttributes(attribute.Int("mcp.tool.count", len(tools)))
	return tools, dialed, nil
}
