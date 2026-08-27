package mcp

import (
	"context"
	"errors"
	"fmt"
	"slices"

	toolcontract "github.com/Tangerg/scope/core/tool"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/scope/app/runtime/internal/domain/mcpserver"
	scopemcp "github.com/Tangerg/scope/mcp"
)

// tracer emits the MCP dial / reconnect spans the lower layers don't (per-call
// MCP tool spans come from the mcp module itself). No-op until a provider is
// installed.
var tracer = otel.Tracer("scope/scopeapp/infra/mcp")

// Dial connects to each configured server, lists its tools, and returns the
// Connections handle alongside the merged model-facing tool list. The server
// name namespaces tools across servers.
//
// Failure, two tiers: a config mistake (duplicate name / invalid entry) is
// FATAL (validated before any dial); a reachability failure is TOLERATED
// (recorded "failed" and skipped). An empty config still yields a live,
// initially-empty Connections so runtime configuration can add servers later.
func Dial(
	ctx context.Context,
	lifetime context.Context,
	servers []ServerConfig,
	oauthSessions OAuthSessionStore,
) (*Connections, []toolcontract.Tool, error) {
	if ctx == nil {
		return nil, nil, errors.New("mcp: startup context is required")
	}
	if lifetime == nil {
		return nil, nil, errors.New("mcp: lifetime is required")
	}
	// Always carry a client, even with zero servers: the registry starts empty
	// and the common path is a 0-server boot followed by a runtime Configure,
	// which re-dials with this client.
	if len(servers) == 0 {
		return &Connections{
			lifetime: lifetime, client: newClient(), oauthSessions: oauthSessions,
		}, nil, nil
	}
	servers = slices.Clone(servers)
	for i := range servers {
		servers[i] = servers[i].Clone()
	}

	// Validate config before dialing: duplicate names collide tool prefixes and
	// a malformed entry can never work — operator mistakes that should fail
	// loudly at boot, not degrade to a "failed" row.
	seen := make(map[string]struct{}, len(servers))
	for index := range servers {
		srv := &servers[index]
		if _, dup := seen[srv.Name]; dup {
			return nil, nil, fmt.Errorf("mcp: duplicate server name %q", srv.Name)
		}
		seen[srv.Name] = struct{}{}
		if verr := srv.Validate(); verr != nil {
			return nil, nil, fmt.Errorf("mcp: invalid server %q: %w", srv.Name, verr)
		}
		if srv.Transport == TransportHTTP && srv.OAuthHandler == nil && !srv.hasStaticAuthorization() {
			handler, err := restoreOAuthHandler(ctx, lifetime, oauthSessions, srv.Name, srv.Endpoint)
			if err != nil {
				return nil, nil, err
			}
			srv.OAuthHandler = handler
		}
	}

	// One client identity for every server — none of scopeapp's connections need
	// per-server handlers (sampling / list-changed), so they share it. Retained
	// so Reconnect / Configure can re-dial with it.
	client := newClient()
	c := &Connections{lifetime: lifetime, client: client, oauthSessions: oauthSessions}

	ctx, span := tracer.Start(ctx, "mcp.dial_servers",
		trace.WithAttributes(attribute.Int("mcp.server.count", len(servers))))
	defer span.End()

	var tools []toolcontract.Tool
	failures := 0
	for _, srv := range servers {
		configuredServer := &server{config: srv, oauth: srv.OAuthHandler}
		configuredServer.config.OAuthHandler = nil
		session, cleanupSession, derr := dial(ctx, lifetime, client, srv)
		if derr != nil {
			configuredServer.state = dialStatus(derr)
			if configuredServer.state == mcpserver.ConnectionNeedsAuth {
				configuredServer.oauth = nil
			}
			failures++
			c.servers = append(c.servers, configuredServer)
			continue
		}
		c.ownSessionLocked(session, cleanupSession)
		srcTools, terr := sourceTools(ctx, scopemcp.ToolSource{Name: srv.Name, Session: session})
		if terr == nil {
			terr = validateToolCatalog(c.servers, nil, srv.Name, srcTools)
		}
		if terr != nil {
			// A session that cannot produce a valid tool catalog is unusable.
			// Preserve a close failure in the trace as well as the primary cause;
			// boot deliberately degrades this one server to failed rather than
			// aborting every independent MCP connection.
			failure := errors.Join(terr, c.closeSession(ctx, session))
			span.RecordError(failure)
			configuredServer.state = mcpserver.ConnectionFailed
			failures++
			c.servers = append(c.servers, configuredServer)
			continue
		}
		configuredServer.session, configuredServer.tools, configuredServer.state = session, srcTools, mcpserver.ConnectionConnected
		tools = append(tools, srcTools...)
		c.servers = append(c.servers, configuredServer)
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
