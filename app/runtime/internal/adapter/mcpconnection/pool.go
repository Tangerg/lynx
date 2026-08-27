// Package mcpconnection adapts persisted MCP server definitions to the live
// MCP connection pool. It is the only runtime layer that knows both the domain
// registry shape and the infrastructure dial shape.
package mcpconnection

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	toolcontract "github.com/Tangerg/scope/core/tool"

	mcpapp "github.com/Tangerg/scope/app/runtime/internal/application/mcp"
	"github.com/Tangerg/scope/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/scope/app/runtime/internal/infra/mcp"
)

// Pool owns the live MCP connections and implements the application ports that
// operate on it. The domain is intentionally passed through as Server values;
// conversion into process, environment, and transport details happens here.
type Pool struct {
	inner *mcp.Connections
}

var (
	_ mcpapp.StatusReader        = (*Pool)(nil)
	_ mcpapp.ToolCatalog         = (*Pool)(nil)
	_ mcpapp.ConnectionControl   = (*Pool)(nil)
	_ mcpapp.ConnectionLifecycle = (*Pool)(nil)
)

// Open establishes the enabled MCP connections present at runtime startup.
// Unreachable but valid servers remain in the pool as failed, matching the
// infrastructure pool's normal boot semantics.
func Open(
	ctx context.Context,
	lifetime context.Context,
	servers []mcpserver.Server,
	oauthSessions mcp.OAuthSessionStore,
) (*Pool, []toolcontract.Tool, error) {
	configs, err := configsFromServers(servers)
	if err != nil {
		return nil, nil, err
	}
	inner, toolset, err := mcp.Dial(ctx, lifetime, configs, oauthSessions)
	if err != nil {
		return nil, nil, err
	}
	return &Pool{inner: inner}, toolset, nil
}

func (p *Pool) Statuses() []mcpserver.ConnectionStatus {
	if p == nil || p.inner == nil {
		return nil
	}
	return p.inner.Statuses()
}

func (p *Pool) Tools(ctx context.Context, server string) ([]mcpserver.AdvertisedTool, error) {
	if p == nil || p.inner == nil {
		return nil, nil
	}
	items, err := p.inner.Tools(ctx, server)
	return items, mapError(err)
}

func (p *Pool) Reconnect(ctx context.Context, name string) error {
	if p == nil || p.inner == nil {
		return mcpserver.ErrUnknownServer
	}
	return mapError(p.inner.Reconnect(ctx, name))
}

func (p *Pool) Authorize(ctx context.Context, name string) error {
	if p == nil || p.inner == nil {
		return mcpserver.ErrUnknownServer
	}
	return mapError(p.inner.Authorize(ctx, name))
}

func (p *Pool) Probe(ctx context.Context, server mcpserver.Server) error {
	if p == nil || p.inner == nil {
		return mcpserver.ErrUnknownServer
	}
	cfg, err := configFromServer(server)
	if err != nil {
		return err
	}
	return mapError(p.inner.Probe(ctx, cfg))
}

func (p *Pool) Configure(ctx context.Context, server mcpserver.Server) error {
	if p == nil || p.inner == nil {
		return mcpserver.ErrUnknownServer
	}
	cfg, err := configFromServer(server)
	if err != nil {
		return err
	}
	return mapError(p.inner.Configure(ctx, cfg))
}

func (p *Pool) Detach(name string) error {
	if p == nil || p.inner == nil {
		return mcp.ErrConnectionsUnavailable
	}
	return mapError(p.inner.Detach(name))
}

// SetToolSink wires live connection changes to the resolver's atomically
// replaceable MCP tool catalog.
func (p *Pool) SetToolSink(sink func([]toolcontract.Tool)) {
	if p == nil || p.inner == nil {
		return
	}
	p.inner.SetToolSink(sink)
}

// Shutdown releases every live connection under the caller's shutdown budget.
func (p *Pool) Shutdown(ctx context.Context) error {
	if p == nil || p.inner == nil {
		return nil
	}
	return p.inner.Shutdown(ctx)
}

func configsFromServers(servers []mcpserver.Server) ([]mcp.ServerConfig, error) {
	if len(servers) == 0 {
		return nil, nil
	}
	out := make([]mcp.ServerConfig, len(servers))
	for i, server := range servers {
		cfg, err := configFromServer(server)
		if err != nil {
			return nil, fmt.Errorf("mcp connection: map server %q: %w", server.Name, err)
		}
		out[i] = cfg
	}
	return out, nil
}

func configFromServer(server mcpserver.Server) (mcp.ServerConfig, error) {
	if err := server.Validate(); err != nil {
		return mcp.ServerConfig{}, fmt.Errorf("validate domain server: %w", err)
	}
	transport, err := transportFromDomain(server.Transport)
	if err != nil {
		return mcp.ServerConfig{}, err
	}
	cfg := mcp.ServerConfig{
		Name:      server.Name,
		Transport: transport,
		Timeout:   server.Timeout,
	}
	switch server.Transport {
	case mcpserver.TransportStreamableHTTP:
		cfg.Endpoint = server.URL
		cfg.Authorization = server.Authorization
		cfg.Headers = maps.Clone(server.Headers)
	case mcpserver.TransportStdio:
		cfg.Command = server.Command
		cfg.Args = slices.Clone(server.Args)
		cfg.Env = flattenEnv(server.SafeEnv())
		cfg.Dir = server.Dir
	}
	if err := cfg.Validate(); err != nil {
		return mcp.ServerConfig{}, fmt.Errorf("validate runtime config: %w", err)
	}
	return cfg, nil
}

func transportFromDomain(transport mcpserver.Transport) (mcp.Transport, error) {
	switch transport {
	case mcpserver.TransportStreamableHTTP:
		return mcp.TransportHTTP, nil
	case mcpserver.TransportStdio:
		return mcp.TransportStdio, nil
	default:
		return "", fmt.Errorf("unknown domain transport %q", transport)
	}
}

func flattenEnv(values map[string]string) []string {
	if len(values) == 0 {
		return nil
	}
	entries := make([]string, 0, len(values))
	for key, value := range values {
		entries = append(entries, key+"="+value)
	}
	slices.Sort(entries)
	return entries
}

func mapError(err error) error {
	if errors.Is(err, mcp.ErrUnknownServer) {
		return fmt.Errorf("%w: %w", mcpserver.ErrUnknownServer, err)
	}
	return err
}
