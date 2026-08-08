// Package mcp coordinates durable server configuration, live connections, and
// the tool policy consumed by run.
package mcp

import (
	"context"
	"sync"

	"github.com/Tangerg/lynx/app/runtime/internal/component/taskgroup"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

// StatusReader reads the live status projection for configured MCP servers.
type StatusReader interface {
	Statuses() []mcpserver.ConnectionStatus
}

// ToolCatalog reads tools advertised by live MCP connections.
type ToolCatalog interface {
	Tools(ctx context.Context, server string) ([]mcpserver.ToolInfo, error)
}

// ConnectionControl reconnects and authorizes configured servers.
// Implementations must sequence operations per server: a newer configure,
// remove, reconnect, or authorize supersedes an older in-flight operation, while
// operations for different servers may proceed concurrently. Each call blocks
// until its live status has settled and honors ctx cancellation; the application
// owns detachment, lifecycle, and asynchronous result publication.
type ConnectionControl interface {
	Reconnect(ctx context.Context, name string) error
	Authorize(ctx context.Context, name string) error
}

// ConnectionLifecycle projects durable server changes into live connections.
type ConnectionLifecycle interface {
	Probe(ctx context.Context, server mcpserver.Server) error
	Configure(ctx context.Context, server mcpserver.Server) error
	Detach(name string) error
}

// Registry is the durable server configuration owned by this application boundary.
type Registry interface {
	List(ctx context.Context) ([]mcpserver.Server, error)
	Get(ctx context.Context, name string) (mcpserver.Server, bool, error)
	Save(ctx context.Context, server mcpserver.Server) error
	Remove(ctx context.Context, name string) error
}

// Coordinator owns durable server configuration, live connections, and the
// atomically published tool policy.
type Coordinator struct {
	// mutationMu linearizes durable registry -> policy/live reconciliation and
	// the short pre/post boundaries of asynchronous connection operations.
	// Network and interactive OAuth waits never hold it; ConnectionControl owns
	// per-server latest-operation-wins sequencing.
	registry              Registry
	statusReader          StatusReader
	toolCatalog           ToolCatalog
	connectionControl     ConnectionControl
	connectionLifecycle   ConnectionLifecycle
	policy                *ToolPolicyState
	mutationMu            sync.Mutex
	dialMu                sync.Mutex
	dials                 map[string]*dial
	statusSequence        uint64
	statusQueue           *statusQueue
	authorizationAttempts *authorizationAttemptStore

	// tasks is this component's context for post-commit reconcile: MCP registry
	// mutations outlive the request but are canceled and joined by the
	// BeginShutdown/AwaitShutdown lifecycle (§10.2 component context, §10.3).
	tasks taskgroup.Group
}

// Config bundles the Coordinator's dependencies.
type Config struct {
	Registry            Registry
	StatusReader        StatusReader
	ToolCatalog         ToolCatalog
	ConnectionControl   ConnectionControl
	ConnectionLifecycle ConnectionLifecycle
	Policy              *ToolPolicyState
	// StatusChanged publishes safe connection status read models. nil disables
	// notification.
	StatusChanged func(status ServerStatus)
}

// New returns a Coordinator over cfg.
func New(cfg Config) *Coordinator {
	if cfg.Policy == nil {
		cfg.Policy = NewToolPolicyState(mcpserver.ToolPolicy{})
	}
	return &Coordinator{
		registry:              cfg.Registry,
		statusReader:          cfg.StatusReader,
		toolCatalog:           cfg.ToolCatalog,
		connectionControl:     cfg.ConnectionControl,
		connectionLifecycle:   cfg.ConnectionLifecycle,
		policy:                cfg.Policy,
		dials:                 make(map[string]*dial),
		statusQueue:           newStatusQueue(cfg.StatusChanged),
		authorizationAttempts: newAuthorizationAttemptStore(),
	}
}

type dial struct {
	cancel context.CancelFunc
}

// BeginShutdown cancels this component's post-commit reconcile work.
// Idempotent; safe to call on a nil Coordinator.
func (c *Coordinator) BeginShutdown() {
	if c == nil {
		return
	}
	c.tasks.Cancel()
}

// AwaitShutdown joins post-commit reconcile work after [BeginShutdown].
func (c *Coordinator) AwaitShutdown(ctx context.Context) error {
	if c == nil {
		return nil
	}
	return c.tasks.Wait(ctx)
}
