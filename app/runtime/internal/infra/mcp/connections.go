package mcp

import (
	"context"
	"maps"
	"slices"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/auth"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/tools"
)

// server is the live state of one configured MCP server. Access is guarded by
// Connections.mu; generation/cancel give each server latest-operation-wins
// semantics without serializing unrelated servers or waiting OAuth flows.
type server struct {
	config  ServerConfig
	session *sdkmcp.ClientSession // nil when not connected
	tools   []tools.Tool          // last tool set proved on this session
	state   mcpserver.ConnectionState
	lastErr error

	// oauth is the live OAuth handler obtained by a successful [Connections.
	// Authorize] this session. nil until the user signs in (an OAuth server with
	// no handler dials anonymously → 401 → needsAuth). Reused on reconnect /
	// reconfigure so a signed-in session stays authorized without re-prompting;
	// not persisted, so a restart clears it.
	oauth auth.OAuthHandler

	generation uint64
	cancel     context.CancelFunc
}

func (s *server) name() string { return s.config.Name }

// Connections owns the live MCP server sessions + reconnect. The optional tool
// sink is invoked with the rebuilt model-facing tool set after a reconnect, so
// the engine can hot-swap the live set into its resolver.
type Connections struct {
	mu        sync.Mutex
	servers   []*server
	client    *sdkmcp.Client
	onTools   func([]tools.Tool) // tool sink; nil until SetToolSink; guarded by mu
	closed    bool               // terminal state set by Close
	closeDone chan struct{}
	closeErr  error

	// publishMu serializes snapshot+sink publication. Mutations themselves run
	// concurrently per server; taking this lock before snapshotting guarantees a
	// delayed publisher can only publish the latest state, never overwrite a
	// newer catalog with an older snapshot.
	publishMu sync.Mutex

	// attempts joins every in-flight dial/OAuth operation during Close. Add is
	// performed under mu before closed can be set, so no Add races the Wait.
	attempts sync.WaitGroup
}

// SetToolSink registers the callback connection mutations invoke with the
// rebuilt model-facing MCP tool set (the engine wires it to its resolver's
// hot-swap).
func (c *Connections) SetToolSink(sink func([]tools.Tool)) {
	c.mu.Lock()
	c.onTools = sink
	c.mu.Unlock()
}

// newClient builds the shared MCP client identity used for every server's
// session (and re-dials). No per-server handlers are needed, so one suffices.
func newClient() *sdkmcp.Client {
	return sdkmcp.NewClient(&sdkmcp.Implementation{Name: "runtime", Version: "v0.1.0"}, nil)
}

// find returns the server with the given name, or nil. Caller holds mu.
func (c *Connections) find(name string) *server {
	for _, ms := range c.servers {
		if ms.name() == name {
			return ms
		}
	}
	return nil
}

func cloneServerConfig(cfg ServerConfig) ServerConfig {
	cfg.Args = slices.Clone(cfg.Args)
	cfg.Env = slices.Clone(cfg.Env)
	cfg.Headers = maps.Clone(cfg.Headers)
	return cfg
}
