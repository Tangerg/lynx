package engine

import (
	"context"
	"errors"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/mcp"
)

// MCPServer is one external MCP server Lyra should connect to at
// startup. Each entry produces a [mcp.Source] whose tools get
// merged into the engine's tool list, prefixed with the server's
// Name so cross-server collisions stay separable.
//
// Endpoint must be a Streamable HTTP MCP URL (the modern transport,
// MCP spec 2025-03-26+). Stdio / subprocess transports are deferred
// to a follow-up — they'd require process supervision the engine
// doesn't currently own.
type MCPServer struct {
	// Name labels the server in tool prefixes and error messages.
	// Required and unique across MCPServers — duplicates fail
	// engine.New so the conflict surfaces at startup, not on the
	// first ambiguous tool call.
	Name string

	// Endpoint is the Streamable HTTP MCP URL.
	// Example: "https://mcp.example.com/".
	Endpoint string
}

// validate ensures MCPServer carries the minimum required fields.
// Engine.New aggregates these checks into a single error so the
// operator sees every misconfiguration at once.
func (m MCPServer) validate() error {
	if m.Name == "" {
		return errors.New("MCPServer.Name is required")
	}
	if m.Endpoint == "" {
		return fmt.Errorf("MCPServer %q: Endpoint is required", m.Name)
	}
	return nil
}

// dialMCPServers connects to each configured MCP server, lists
// tools, and returns the merged tool list alongside the open
// sessions so the caller can close them at shutdown.
//
// Failure semantics: any single server that can't be reached
// fails the whole call — the operator should see the misconfig
// up front rather than discover later that a tool went missing
// silently. Already-dialed sessions are closed before returning
// the error so we don't leak subprocesses or HTTP connections.
func dialMCPServers(ctx context.Context, servers []MCPServer) ([]chat.Tool, []*sdkmcp.ClientSession, error) {
	if len(servers) == 0 {
		return nil, nil, nil
	}

	seenNames := make(map[string]struct{}, len(servers))
	sources := make([]mcp.Source, 0, len(servers))
	sessions := make([]*sdkmcp.ClientSession, 0, len(servers))
	closeAll := func() {
		for _, sess := range sessions {
			_ = sess.Close()
		}
	}

	for _, srv := range servers {
		if err := srv.validate(); err != nil {
			closeAll()
			return nil, nil, err
		}
		if _, dup := seenNames[srv.Name]; dup {
			closeAll()
			return nil, nil, fmt.Errorf("engine: duplicate MCPServer name %q", srv.Name)
		}
		seenNames[srv.Name] = struct{}{}

		client := sdkmcp.NewClient(&sdkmcp.Implementation{
			Name:    "lyra",
			Version: "v0.1.0",
		}, nil)
		session, err := mcp.DialStreamableHTTP(ctx, client, srv.Endpoint, nil)
		if err != nil {
			closeAll()
			return nil, nil, fmt.Errorf("engine: dial MCP server %q: %w", srv.Name, err)
		}
		sessions = append(sessions, session)
		sources = append(sources, mcp.Source{Name: srv.Name, Session: session})
	}

	provider, err := mcp.NewProvider(&mcp.ProviderConfig{
		Sources: sources,
	})
	if err != nil {
		closeAll()
		return nil, nil, fmt.Errorf("engine: build MCP provider: %w", err)
	}

	tools, err := provider.Tools(ctx)
	if err != nil {
		closeAll()
		return nil, nil, fmt.Errorf("engine: list MCP tools: %w", err)
	}

	return tools, sessions, nil
}
