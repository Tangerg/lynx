// Package capabilities is the application coordinator for the runtime's
// capability + configuration surface: the tool-permission stance (approval),
// the diagnostic tool registry, the provider registry + static catalog, the
// model roles (utility / embedding), and the default provider/model. It is a
// thin use-case layer over the domain services and a few composition-injected
// ports (client/embedding resolvers, provider catalog + prober); the delivery
// layer drives it per settings/capability request.
package capabilities

import (
	"context"
	"sync/atomic"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelrole"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	toolsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// SessionLookup resolves a session so approval-rule listing can scope rules to
// the session's project directory. The session store satisfies it.
type SessionLookup interface {
	Get(ctx context.Context, id string) (session.Session, error)
}

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

// Coordinator owns the capability + configuration use cases. Any nil dependency
// disables the corresponding capability.
type Coordinator struct {
	approval  approval.Policy
	tools     toolsvc.Registry
	providers provider.Registry
	catalog   ProviderCatalog
	prober    ProviderProber
	sessions  SessionLookup

	// utility / embedding model roles: the live cell (shared with the maintenance
	// titler / codebase index that read it), the resolver that validates a new
	// role, and the saver that persists it.
	utilityCell     *atomic.Pointer[modelrole.Role]
	utilityResolver ClientResolver
	utilityStore    UtilityRoleSaver

	embeddingCell     *atomic.Pointer[modelrole.Role]
	embeddingResolver EmbeddingResolver
	embeddingStore    EmbeddingRoleSaver

	defaultProvider string
	defaultModel    string
}

// Config bundles the Coordinator's dependencies.
type Config struct {
	Approval  approval.Policy
	Tools     toolsvc.Registry
	Providers provider.Registry
	Catalog   ProviderCatalog
	Prober    ProviderProber
	Sessions  SessionLookup

	UtilityCell     *atomic.Pointer[modelrole.Role]
	UtilityResolver ClientResolver
	UtilityStore    UtilityRoleSaver

	EmbeddingCell     *atomic.Pointer[modelrole.Role]
	EmbeddingResolver EmbeddingResolver
	EmbeddingStore    EmbeddingRoleSaver

	DefaultProvider string
	DefaultModel    string
}

// New returns a capabilities Coordinator over cfg.
func New(cfg Config) *Coordinator {
	return &Coordinator{
		approval:          cfg.Approval,
		tools:             cfg.Tools,
		providers:         cfg.Providers,
		catalog:           cfg.Catalog,
		prober:            cfg.Prober,
		sessions:          cfg.Sessions,
		utilityCell:       cfg.UtilityCell,
		utilityResolver:   cfg.UtilityResolver,
		utilityStore:      cfg.UtilityStore,
		embeddingCell:     cfg.EmbeddingCell,
		embeddingResolver: cfg.EmbeddingResolver,
		embeddingStore:    cfg.EmbeddingStore,
		defaultProvider:   cfg.DefaultProvider,
		defaultModel:      cfg.DefaultModel,
	}
}
