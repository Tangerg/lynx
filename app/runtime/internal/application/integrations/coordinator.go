// Package integrations is the application coordinator for the runtime's external
// integration surface — currently MCP: the durable MCP server registry (source
// of truth), its live connection pool, and the atomically-published tool policy
// the execution tool gate reads. It is a thin use-case layer over domain services
// the delivery mcp.* handlers drive.
package integrations

import (
	"context"
	"sync"

	"github.com/Tangerg/lynx/app/runtime/internal/component/taskgroup"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

// MCPStatusReader reads the live status projection for configured MCP servers.
type MCPStatusReader interface {
	Statuses() []mcpserver.ConnectionStatus
}

// MCPToolCatalog reads tools advertised by live MCP connections.
type MCPToolCatalog interface {
	Tools(ctx context.Context, server string) ([]mcpserver.ToolInfo, error)
}

// MCPConnectionCommands reconnects and authorizes configured MCP servers.
// Implementations must sequence operations per server: a newer configure,
// remove, reconnect, or authorize supersedes an older in-flight operation, while
// operations for different servers may proceed concurrently.
type MCPConnectionCommands interface {
	Reconnect(ctx context.Context, name string) error
	Authorize(ctx context.Context, name string) error
}

// MCPRegistryCommands projects durable registry changes into the live MCP pool.
type MCPRegistryCommands interface {
	Probe(ctx context.Context, server mcpserver.Server) error
	Configure(ctx context.Context, server mcpserver.Server) error
	Remove(ctx context.Context, name string)
}

// Registry is the durable MCP-server surface the integration use cases own.
type Registry interface {
	List(ctx context.Context) ([]mcpserver.Server, error)
	Get(ctx context.Context, name string) (mcpserver.Server, bool, error)
	Configure(ctx context.Context, server mcpserver.Server) error
	Remove(ctx context.Context, name string) error
	SetEnabled(ctx context.Context, name string, enabled bool) error
}

// Coordinator owns the MCP integration use cases: the durable server registry,
// its live connection pool, and the atomically-published tool policy.
type Coordinator struct {
	// MCP: the durable registry (source of truth), the live connection pool
	// (projection), and the atomically-published ToolPolicy the engine's tool gate
	// + approval read. mcpMutationMu linearizes durable registry -> policy/live
	// reconciliation and the short pre/post boundaries of asynchronous connect
	// operations. Network and interactive OAuth waits never hold it; the live
	// adapter owns per-server latest-operation-wins sequencing.
	mcpRegistry           Registry
	mcpStatusReader       MCPStatusReader
	mcpToolCatalog        MCPToolCatalog
	mcpConnectionCommands MCPConnectionCommands
	mcpRegistryCommands   MCPRegistryCommands
	mcpPolicy             *ToolPolicyState
	mcpMutationMu         sync.Mutex
	mcpDialMu             sync.Mutex
	mcpDials              map[string]*mcpDial

	// tasks is this component's context for post-commit reconcile: MCP registry
	// mutations outlive the request but are canceled + joined by Close (§10.2
	// component context, §10.3).
	tasks taskgroup.Group

	// mcpStatus publishes fully resolved, safe MCP status read models so a
	// delivery consumer can republish them without reaching back into live
	// infrastructure. The composition root injects the notifier's Publish.
	mcpStatus func(status MCPServerStatus)
}

// Config bundles the Coordinator's dependencies.
type Config struct {
	MCPRegistry           Registry
	MCPStatusReader       MCPStatusReader
	MCPToolCatalog        MCPToolCatalog
	MCPConnectionCommands MCPConnectionCommands
	MCPRegistryCommands   MCPRegistryCommands
	MCPPolicy             *ToolPolicyState
	// MCPStatus publishes safe MCP connection status read models to the delivery
	// workspace stream bridge. nil disables notification.
	MCPStatus func(status MCPServerStatus)
}

// New returns an integrations Coordinator over cfg.
func New(cfg Config) *Coordinator {
	if cfg.MCPPolicy == nil {
		cfg.MCPPolicy = NewToolPolicyState(mcpserver.ToolPolicy{})
	}
	return &Coordinator{
		mcpRegistry:           cfg.MCPRegistry,
		mcpStatusReader:       cfg.MCPStatusReader,
		mcpToolCatalog:        cfg.MCPToolCatalog,
		mcpConnectionCommands: cfg.MCPConnectionCommands,
		mcpRegistryCommands:   cfg.MCPRegistryCommands,
		mcpPolicy:             cfg.MCPPolicy,
		mcpStatus:             cfg.MCPStatus,
		mcpDials:              make(map[string]*mcpDial),
	}
}

type mcpDial struct {
	cancel context.CancelFunc
}

// Close cancels and joins this component's post-commit reconcile work (§10.3).
// Idempotent; safe to call on a nil Coordinator.
func (c *Coordinator) Close() {
	if c == nil {
		return
	}
	c.tasks.Close()
}
