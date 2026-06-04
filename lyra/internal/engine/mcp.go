package engine

import (
	"context"
	"fmt"
	"log/slog"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/mcp"
)

// engineTracer emits the lyra-runtime startup / orchestration spans the
// lower layers don't (MCP dial). Per-call MCP tool spans come from the
// mcp module itself. No-op until a TracerProvider is installed.
var engineTracer = otel.Tracer("lynx/lyra/engine")

// dialMCPServers connects to each configured MCP server, lists tools,
// and returns the merged tool list alongside the open sessions so the
// caller can close them at shutdown. Each session is wrapped in an
// [mcp.Source] named after its config so tools stay namespaced across
// servers.
//
// Failure semantics: any single server that can't be reached (or is
// misconfigured — [mcp.Dial] validates the config) fails the whole call,
// so the operator sees the problem at startup rather than discovering a
// tool went missing silently. Already-dialed sessions are closed before
// returning the error so we don't leak subprocesses or HTTP connections.
func dialMCPServers(ctx context.Context, servers []mcp.ServerConfig) (tools []chat.Tool, sessions []*sdkmcp.ClientSession, err error) {
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
	sources := make([]mcp.Source, 0, len(servers))
	sessions = make([]*sdkmcp.ClientSession, 0, len(servers))
	closeAll := func() {
		for _, sess := range sessions {
			_ = sess.Close()
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
		sessions = append(sessions, session)
		sources = append(sources, mcp.Source{Name: srv.Name, Session: session})
		slog.InfoContext(ctx, "mcp server connected", "mcp.server.name", srv.Name)
	}

	provider, perr := mcp.NewProvider(mcp.ProviderConfig{Sources: sources})
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
	return tools, sessions, nil
}
