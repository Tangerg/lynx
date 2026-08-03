package mcp

import (
	"context"
	"maps"
	"slices"
	"sync"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/modelcontextprotocol/go-sdk/auth"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

// server is the live state of one configured MCP server. Access is guarded by
// Connections.mu; generation/cancel give each server latest-operation-wins
// semantics without serializing unrelated servers or waiting OAuth flows.
type server struct {
	config  ServerConfig
	session *sdkmcp.ClientSession // nil when not connected
	tools   []toolcontract.Tool   // last tool set proved on this session
	state   mcpserver.ConnectionState

	// oauth is either a restored durable OAuth handler or the handler obtained by
	// a successful [Connections.Authorize]. nil until a saved session exists or
	// the user signs in. It is reusable only within the same HTTP origin.
	oauth auth.OAuthHandler

	generation uint64
	cancel     context.CancelFunc
}

func (s *server) name() string { return s.config.Name }

type shutdownAttempt struct {
	done chan struct{}
	err  error
}

type sessionCloseAttempt struct {
	done chan struct{}
	err  error
}

type ownedSession struct {
	closeFn func() error
	close   *sessionCloseAttempt
}

// Connections owns the live MCP server sessions + reconnect. The optional tool
// sink is invoked with the rebuilt model-facing tool set after a reconnect, so
// the engine can hot-swap the live set into its resolver.
type Connections struct {
	mu       sync.Mutex
	servers  []*server
	client   *sdkmcp.Client
	onTools  func([]toolcontract.Tool) // tool sink; nil until SetToolSink; guarded by mu
	closed   bool                      // terminal admission state set by Shutdown
	shutdown *shutdownAttempt
	sessions map[*sdkmcp.ClientSession]*ownedSession

	// oauthSessions is the durable credential boundary. It is optional so the
	// infrastructure remains usable in processes that deliberately opt out of
	// persistence; the desktop runtime always supplies it.
	oauthSessions OAuthSessionStore

	// publishMu serializes snapshot+sink publication. Mutations themselves run
	// concurrently per server; taking this lock before snapshotting guarantees a
	// delayed publisher can only publish the latest state, never overwrite a
	// newer catalog with an older snapshot.
	publishMu sync.Mutex

	// attempts joins every in-flight dial/OAuth operation during Shutdown. Add is
	// performed under mu before closed can be set, so no Add races the Wait.
	attempts sync.WaitGroup
}

// SetToolSink registers the callback connection mutations invoke with the
// rebuilt model-facing MCP tool set (the engine wires it to its resolver's
// hot-swap).
func (c *Connections) SetToolSink(sink func([]toolcontract.Tool)) {
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

func (c *Connections) ownSessionLocked(session *sdkmcp.ClientSession) {
	if session == nil {
		return
	}
	if c.sessions == nil {
		c.sessions = make(map[*sdkmcp.ClientSession]*ownedSession)
	}
	if c.sessions[session] == nil {
		c.sessions[session] = &ownedSession{closeFn: session.Close}
	}
}

func cloneServerConfig(cfg ServerConfig) ServerConfig {
	cfg.Args = slices.Clone(cfg.Args)
	cfg.Env = slices.Clone(cfg.Env)
	cfg.Headers = maps.Clone(cfg.Headers)
	return cfg
}
