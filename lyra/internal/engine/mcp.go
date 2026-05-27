package engine

import (
	"context"
	"errors"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/mcp"
)

// MCPTransport is the wire mode for one [MCPServer] entry. Empty
// (zero value) is illegal; engine.New rejects ambiguous entries up
// front so misconfiguration surfaces at startup, not on the first
// tool call.
type MCPTransport int

const (
	// MCPTransportHTTP — Streamable HTTP MCP server. Endpoint is the
	// URL; Command / Args / Env are ignored.
	MCPTransportHTTP MCPTransport = iota + 1
	// MCPTransportStdio — local subprocess spawned by Command + Args,
	// communicating over stdin/stdout (the canonical local-tool
	// transport; most ecosystem MCP servers ship as
	// `npx -y @modelcontextprotocol/server-<name>` commands).
	MCPTransportStdio
)

// MCPServer is one external MCP server Lyra should connect to at
// startup. Each entry produces a [mcp.Source] whose tools merge
// into the engine's tool list, prefixed with the server's Name so
// cross-server collisions stay separable.
//
// Exactly one transport field set is required:
//
//   - Transport == HTTP: Endpoint must be a non-empty URL.
//   - Transport == Stdio: Command must be non-empty; Args / Env / Dir
//     are optional.
type MCPServer struct {
	// Name labels the server in tool prefixes and error messages.
	// Required and unique across MCPServers — duplicates fail
	// engine.New so the conflict surfaces at startup, not on the
	// first ambiguous tool call.
	Name string

	// Transport picks the connection mode. Defaults to
	// [MCPTransportHTTP] when zero (so the older "name + endpoint"
	// callers stay valid).
	Transport MCPTransport

	// Endpoint is the Streamable HTTP MCP URL.
	// Used when Transport == [MCPTransportHTTP].
	// Example: "https://mcp.example.com/".
	Endpoint string

	// Command is the executable to spawn for a stdio MCP server.
	// Used when Transport == [MCPTransportStdio].
	// Example: "npx".
	Command string

	// Args are the command arguments for a stdio MCP server.
	// Used when Transport == [MCPTransportStdio].
	// Example: ["-y", "@modelcontextprotocol/server-filesystem", "/workspace"].
	Args []string

	// Env, when non-nil, replaces the subprocess environment (it
	// does not extend the parent env — append os.Environ() yourself
	// when extension is wanted). Used when Transport ==
	// [MCPTransportStdio].
	Env []string

	// Dir sets the subprocess working directory. Empty inherits the
	// parent's cwd. Used when Transport == [MCPTransportStdio].
	Dir string
}

// validate ensures one transport is fully specified and any others
// are blank. Engine.New aggregates these so the operator sees
// every misconfiguration at once.
func (m MCPServer) Validate() error {
	if m.Name == "" {
		return errors.New("MCPServer.Name is required")
	}
	switch m.transport() {
	case MCPTransportHTTP:
		if m.Endpoint == "" {
			return fmt.Errorf("MCPServer %q: Endpoint is required for HTTP transport", m.Name)
		}
		if m.Command != "" {
			return fmt.Errorf("MCPServer %q: Command must be empty for HTTP transport", m.Name)
		}
	case MCPTransportStdio:
		if m.Command == "" {
			return fmt.Errorf("MCPServer %q: Command is required for stdio transport", m.Name)
		}
		if m.Endpoint != "" {
			return fmt.Errorf("MCPServer %q: Endpoint must be empty for stdio transport", m.Name)
		}
	default:
		return fmt.Errorf("MCPServer %q: unknown transport %d", m.Name, m.Transport)
	}
	return nil
}

// transport normalises the discriminator: zero value falls back to
// HTTP so callers that only set Endpoint (the original API) keep
// working without explicitly setting Transport.
func (m MCPServer) transport() MCPTransport {
	if m.Transport == 0 {
		return MCPTransportHTTP
	}
	return m.Transport
}

// dialMCPServers connects to each configured MCP server, lists
// tools, and returns the merged tool list alongside the open
// sessions so the caller can close them at shutdown.
//
// Failure semantics: any single server that can't be reached fails
// the whole call — the operator should see the misconfig up front
// rather than discover later that a tool went missing silently.
// Already-dialed sessions are closed before returning the error so
// we don't leak subprocesses or HTTP connections.
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
		if err := srv.Validate(); err != nil {
			closeAll()
			return nil, nil, err
		}
		if _, dup := seenNames[srv.Name]; dup {
			closeAll()
			return nil, nil, fmt.Errorf("engine: duplicate MCPServer name %q", srv.Name)
		}
		seenNames[srv.Name] = struct{}{}

		session, err := dialOne(ctx, srv)
		if err != nil {
			closeAll()
			return nil, nil, fmt.Errorf("engine: dial MCP server %q: %w", srv.Name, err)
		}
		sessions = append(sessions, session)
		sources = append(sources, mcp.Source{Name: srv.Name, Session: session})
	}

	provider, err := mcp.NewProvider(mcp.ProviderConfig{
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

// dialOne dispatches to the right transport helper. The client
// `Implementation` is the same across transports — the only thing
// that changes is the transport struct.
func dialOne(ctx context.Context, srv MCPServer) (*sdkmcp.ClientSession, error) {
	client := sdkmcp.NewClient(&sdkmcp.Implementation{
		Name:    "lyra",
		Version: "v0.1.0",
	}, nil)
	switch srv.transport() {
	case MCPTransportHTTP:
		return mcp.DialStreamableHTTP(ctx, client, srv.Endpoint, mcp.HTTPClientOptions{})
	case MCPTransportStdio:
		return mcp.DialCommand(ctx, client, srv.Command, srv.Args, mcp.CommandClientOptions{
			Env: srv.Env,
			Dir: srv.Dir,
		})
	default:
		return nil, fmt.Errorf("unknown transport %d", srv.Transport)
	}
}
