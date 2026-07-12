// Package capabilities is the application coordinator for the runtime's MCP
// integration surface + the diagnostic tool registry: the durable MCP server
// registry (source of truth), its live connection pool, the atomically-published
// tool policy the engine's gate reads, and the read-only registered-tool listing.
// It is a thin use-case layer over the domain services the delivery mcp.* /
// tools.* handlers drive.
package capabilities

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/Tangerg/lynx/app/runtime/internal/component/taskgroup"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	toolsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// MCPLive is the process-local MCP connection pool: the live projection of the
// durable registry (§9). The kernel engine satisfies it. Registry (source of
// truth) vs connection pool (live) stay distinct — this port is only the pool.
type MCPLive interface {
	MCPServerStatuses() []mcpserver.ConnectionStatus
	MCPTools(ctx context.Context, server string) ([]mcpserver.ToolInfo, error)
	ReconnectMCPServer(ctx context.Context, name string) error
	AuthorizeMCPServer(ctx context.Context, name string) error
	ProbeMCPServer(ctx context.Context, cfg mcpserver.LiveConfig) error
	ConfigureMCPServer(ctx context.Context, cfg mcpserver.LiveConfig) error
	RemoveMCPServer(ctx context.Context, name string)
}

// Coordinator owns the MCP integration + tool-registry use cases.
type Coordinator struct {
	tools toolsvc.Registry

	// MCP: the durable registry (source of truth), the live connection pool
	// (projection), and the atomically-published ToolPolicy the engine's tool gate
	// + approval read. mcpMutationMu linearizes the multi-step registry -> live ->
	// policy write; locks inside the store/pool can't span that boundary.
	mcpRegistry   mcpserver.Registry
	mcpLive       MCPLive
	mcpPolicy     *atomic.Pointer[mcpserver.ToolPolicy]
	mcpMutationMu sync.Mutex

	// tasks is this component's context for post-commit reconcile: MCP registry
	// mutations outlive the request but are canceled + joined by Close (§10.2
	// component context, §10.3).
	tasks taskgroup.Group

	// mcpStatus publishes an MCP server's connection transitions (connecting →
	// settled) so a delivery consumer can republish them on the workspace event
	// stream; nil disables the notification (the reconnect still runs). The
	// composition root injects the notifier's Publish.
	mcpStatus func(ctx context.Context, server string, connecting bool)
}

// Config bundles the Coordinator's dependencies.
type Config struct {
	Tools       toolsvc.Registry
	MCPRegistry mcpserver.Registry
	MCPLive     MCPLive
	MCPPolicy   *atomic.Pointer[mcpserver.ToolPolicy]
	// MCPStatus publishes MCP connection transitions to the delivery workspace
	// stream (the notifier's Publish). nil disables the notification.
	MCPStatus func(ctx context.Context, server string, connecting bool)
}

// New returns a capabilities Coordinator over cfg.
func New(cfg Config) *Coordinator {
	return &Coordinator{
		tools:       cfg.Tools,
		mcpRegistry: cfg.MCPRegistry,
		mcpLive:     cfg.MCPLive,
		mcpPolicy:   cfg.MCPPolicy,
		mcpStatus:   cfg.MCPStatus,
	}
}

// Close cancels and joins this component's post-commit reconcile work (§10.3).
// Idempotent; safe to call on a nil Coordinator.
func (c *Coordinator) Close() {
	if c == nil {
		return
	}
	c.tasks.Close()
}
