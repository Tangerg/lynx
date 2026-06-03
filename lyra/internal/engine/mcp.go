package engine

import (
	"context"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/mcp"
)

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
func dialMCPServers(ctx context.Context, servers []mcp.ServerConfig) ([]chat.Tool, []*sdkmcp.ClientSession, error) {
	if len(servers) == 0 {
		return nil, nil, nil
	}

	// One client identity for every server — none of lyra's MCP
	// connections need per-server client handlers (sampling /
	// list-changed), so they share it.
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "runtime", Version: "v0.1.0"}, nil)

	seenNames := make(map[string]struct{}, len(servers))
	sources := make([]mcp.Source, 0, len(servers))
	sessions := make([]*sdkmcp.ClientSession, 0, len(servers))
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

		session, err := mcp.Dial(ctx, client, srv)
		if err != nil {
			closeAll()
			return nil, nil, fmt.Errorf("engine: dial MCP server %q: %w", srv.Name, err)
		}
		sessions = append(sessions, session)
		sources = append(sources, mcp.Source{Name: srv.Name, Session: session})
	}

	provider, err := mcp.NewProvider(mcp.ProviderConfig{Sources: sources})
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
