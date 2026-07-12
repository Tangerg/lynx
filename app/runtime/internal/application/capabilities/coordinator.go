// Package capabilities is the application coordinator for the runtime's
// capability + configuration surface: the diagnostic tool registry, the provider
// registry + static catalog, the model roles (utility / embedding), the default
// provider/model, and the MCP server registry. It is a thin use-case layer over
// the domain services and a few composition-injected ports (client/embedding
// resolvers, provider catalog + prober); the delivery layer drives it per
// settings/capability request.
package capabilities

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/component/taskgroup"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelrole"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
	toolsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// ProviderCatalog is the static provider reference data (which providers this
// build serves + their capabilities), projected from the infra provider table.
// The composition root supplies it (only it may read the infra catalog).
type ProviderCatalog interface {
	Supported() []provider.Metadata
	Metadata(id string) (provider.Metadata, bool)
}

// ProviderProber validates a provider's credentials with one minimal live call
// (providers.test). The composition root supplies it (it owns client
// construction against the infra provider adapters).
type ProviderProber interface {
	Probe(ctx context.Context, entry provider.Provider) error
}

// ClientResolver validates/builds a chat client for (provider, model). The
// utility-role setter uses it to reject an unconfigured role before persisting.
type ClientResolver interface {
	ResolveClient(ctx context.Context, providerID, model string) (*chat.Client, error)
}

// EmbeddingResolver validates/builds an embedder for (provider, model). The
// embedding-role setter uses it to reject an unbuildable role before persisting.
type EmbeddingResolver interface {
	Resolve(ctx context.Context, providerID, model string) (codebaseindex.Embedder, error)
}

// UtilityRoleSaver persists the utility-model role across restarts. nil disables
// persistence (the role stays in-process only).
type UtilityRoleSaver interface {
	SaveUtilityRole(ctx context.Context, provider, model string) error
}

// EmbeddingRoleSaver persists the embedding-model role across restarts. nil
// disables persistence.
type EmbeddingRoleSaver interface {
	SaveEmbeddingRole(ctx context.Context, provider, model string) error
}

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

// Coordinator owns the capability + configuration use cases. Any nil dependency
// disables the corresponding capability.
type Coordinator struct {
	tools     toolsvc.Registry
	providers provider.Registry
	catalog   ProviderCatalog
	prober    ProviderProber

	// utility / embedding model roles: the live cell (shared with the maintenance
	// titler / codebase index that read it), the resolver that validates a new
	// role, and the saver that persists it.
	utilityCell     *atomic.Pointer[modelrole.Role]
	utilityResolver ClientResolver
	utilityStore    UtilityRoleSaver

	embeddingCell     *atomic.Pointer[modelrole.Role]
	embeddingResolver EmbeddingResolver
	embeddingStore    EmbeddingRoleSaver

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

	defaultProvider string
	defaultModel    string
}

// Config bundles the Coordinator's dependencies.
type Config struct {
	Tools     toolsvc.Registry
	Providers provider.Registry
	Catalog   ProviderCatalog
	Prober    ProviderProber

	UtilityCell     *atomic.Pointer[modelrole.Role]
	UtilityResolver ClientResolver
	UtilityStore    UtilityRoleSaver

	EmbeddingCell     *atomic.Pointer[modelrole.Role]
	EmbeddingResolver EmbeddingResolver
	EmbeddingStore    EmbeddingRoleSaver

	MCPRegistry mcpserver.Registry
	MCPLive     MCPLive
	MCPPolicy   *atomic.Pointer[mcpserver.ToolPolicy]
	// MCPStatus publishes MCP connection transitions to the delivery workspace
	// stream (the notifier's Publish). nil disables the notification.
	MCPStatus func(ctx context.Context, server string, connecting bool)

	DefaultProvider string
	DefaultModel    string
}

// New returns a capabilities Coordinator over cfg.
func New(cfg Config) *Coordinator {
	return &Coordinator{
		tools:             cfg.Tools,
		providers:         cfg.Providers,
		catalog:           cfg.Catalog,
		prober:            cfg.Prober,
		utilityCell:       cfg.UtilityCell,
		utilityResolver:   cfg.UtilityResolver,
		utilityStore:      cfg.UtilityStore,
		embeddingCell:     cfg.EmbeddingCell,
		embeddingResolver: cfg.EmbeddingResolver,
		embeddingStore:    cfg.EmbeddingStore,
		mcpRegistry:       cfg.MCPRegistry,
		mcpLive:           cfg.MCPLive,
		mcpPolicy:         cfg.MCPPolicy,
		mcpStatus:         cfg.MCPStatus,
		defaultProvider:   cfg.DefaultProvider,
		defaultModel:      cfg.DefaultModel,
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
