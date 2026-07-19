package mcp

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	lynxmcp "github.com/Tangerg/lynx/mcp"
	"github.com/Tangerg/lynx/tools"
)

// tracer emits the MCP dial / reconnect spans the lower layers don't (per-call
// MCP tool spans come from the mcp module itself). No-op until a provider is
// installed.
var tracer = otel.Tracer("lynx/lyra/infra/mcp")

// Dial connects to each configured server, lists its tools, and returns the
// Connections handle alongside the merged model-facing tool list. The server
// name namespaces tools across servers.
//
// Failure, two tiers: a config mistake (duplicate name / invalid entry) is
// FATAL (validated before any dial); a reachability failure is TOLERATED
// (recorded "failed" and skipped). An empty config still yields a live,
// initially-empty Connections so runtime configuration can add servers later.
func Dial(ctx context.Context, servers []ServerConfig) (*Connections, []tools.Tool, error) {
	// Always carry a client, even with zero servers: the registry starts empty
	// and the common path is a 0-server boot followed by a runtime Configure,
	// which re-dials with this client.
	if len(servers) == 0 {
		return &Connections{client: newClient()}, nil, nil
	}
	servers = slices.Clone(servers)
	for i := range servers {
		servers[i] = cloneServerConfig(servers[i])
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
	// so Reconnect / Configure can re-dial with it.
	client := newClient()
	c := &Connections{client: client}

	ctx, span := tracer.Start(ctx, "mcp.dial_servers",
		trace.WithAttributes(attribute.Int("mcp.server.count", len(servers))))
	defer span.End()

	var tools []tools.Tool
	failures := 0
	for _, srv := range servers {
		ms := &server{config: srv}
		session, derr := dial(ctx, client, srv)
		if derr != nil {
			ms.state, ms.lastErr = dialStatus(derr), derr
			failures++
			c.servers = append(c.servers, ms)
			continue
		}
		srcTools, terr := sourceTools(ctx, lynxmcp.ToolSource{Name: srv.Name, Session: session})
		if terr == nil {
			terr = validateToolCatalog(c.servers, nil, srv.Name, srcTools)
		}
		if terr != nil {
			terr = errors.Join(terr, session.Close()) // half-open: drop an unusable session
			ms.state, ms.lastErr = mcpserver.ConnectionFailed, terr
			failures++
			c.servers = append(c.servers, ms)
			continue
		}
		ms.session, ms.tools, ms.state = session, srcTools, mcpserver.ConnectionConnected
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
