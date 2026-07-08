package mcp

import (
	"context"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	lynxmcp "github.com/Tangerg/lynx/mcp"
)

// Probe tests cfg with a throwaway client (workspace.mcp.test). It reuses an
// active OAuth sign-in for the same-named server this session, because an
// anonymous probe of an OAuth-protected server would always 401 even though the
// live connection is authorized; the probe must carry the session's token to
// reflect reality. Falls back to a stateless (anonymous / bearer / headers)
// probe when there's no live handler. Nil receiver is supported (no live set).
func (c *Connections) Probe(ctx context.Context, cfg ServerConfig) error {
	if cfg.OAuthHandler == nil && c != nil {
		c.mu.Lock()
		if ms := c.find(cfg.Name); ms != nil {
			cfg.OAuthHandler = ms.oauth
		}
		c.mu.Unlock()
	}
	return probe(ctx, cfg)
}

// probe dials cfg with a throwaway client, proves its tools are listable, and
// closes the session; a connection test that touches no live state. Honors any
// cfg.OAuthHandler so a probe can be authorized. Returns nil on success.
func probe(ctx context.Context, cfg ServerConfig) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	ctx, span := tracer.Start(ctx, "mcp.probe",
		trace.WithAttributes(attribute.String("mcp.server.name", cfg.Name)))
	defer span.End()

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "runtime-probe", Version: "v0.1.0"}, nil)
	session, err := dial(ctx, client, cfg)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	defer session.Close()
	if _, err := sourceTools(ctx, lynxmcp.ToolSource{Name: cfg.Name, Session: session}); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}
