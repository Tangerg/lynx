// Package models is the application coordinator for provider + model
// configuration: the runtime-mutable provider registry (credentials), the static
// provider catalog + credential prober, the utility / embedding model roles, and
// the default provider/model a run falls back to. It is a thin use-case layer
// over the domain provider registry + a few composition-injected ports
// (client/embedding resolvers, catalog, prober); the delivery layer drives it per
// providers.* / models.* request.
package models

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelrole"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
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

// ProviderModelLister discovers a provider's available models by probing its
// live endpoint — used for local / bring-your-own-endpoint providers whose model
// set is not in the static catalog (models.list of an Ollama daemon or a compat
// passthrough). The composition root supplies it (it owns endpoint resolution +
// the outbound probe). A nil lister disables live discovery (static catalog only).
type ProviderModelLister interface {
	ListModels(ctx context.Context, entry provider.Provider) ([]string, error)
}

// ChatModelValidator verifies that a chat client can be built for
// (provider, model) without exposing the concrete client to Application.
type ChatModelValidator interface {
	ValidateChatModel(ctx context.Context, providerID, model string) error
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

// Coordinator owns provider + model configuration. Any nil dependency disables
// the corresponding capability.
type Coordinator struct {
	providers provider.Registry
	catalog   ProviderCatalog
	prober    ProviderProber
	lister    ProviderModelLister

	// utility / embedding model roles: the live cell (shared with the maintenance
	// titler / codebase index that read it), the resolver that validates a new
	// role, and the saver that persists it.
	utilityCell      *atomic.Pointer[modelrole.Role]
	utilityValidator ChatModelValidator
	utilityStore     UtilityRoleSaver
	utilityMu        sync.Mutex

	embeddingCell     *atomic.Pointer[modelrole.Role]
	embeddingResolver EmbeddingResolver
	embeddingStore    EmbeddingRoleSaver
	embeddingMu       sync.Mutex

	defaultProvider string
	defaultModel    string
}

// Config bundles the Coordinator's dependencies.
type Config struct {
	Providers provider.Registry
	Catalog   ProviderCatalog
	Prober    ProviderProber
	Lister    ProviderModelLister

	UtilityCell      *atomic.Pointer[modelrole.Role]
	UtilityValidator ChatModelValidator
	UtilityStore     UtilityRoleSaver

	EmbeddingCell     *atomic.Pointer[modelrole.Role]
	EmbeddingResolver EmbeddingResolver
	EmbeddingStore    EmbeddingRoleSaver

	DefaultProvider string
	DefaultModel    string
}

// New returns a models Coordinator over cfg.
func New(cfg Config) *Coordinator {
	return &Coordinator{
		providers:         cfg.Providers,
		catalog:           cfg.Catalog,
		prober:            cfg.Prober,
		lister:            cfg.Lister,
		utilityCell:       cfg.UtilityCell,
		utilityValidator:  cfg.UtilityValidator,
		utilityStore:      cfg.UtilityStore,
		embeddingCell:     cfg.EmbeddingCell,
		embeddingResolver: cfg.EmbeddingResolver,
		embeddingStore:    cfg.EmbeddingStore,
		defaultProvider:   cfg.DefaultProvider,
		defaultModel:      cfg.DefaultModel,
	}
}

// DefaultModel returns the fallback model a run uses when it selects none.
func (c *Coordinator) DefaultModel() string { return c.defaultModel }

// DefaultProvider returns the fallback provider a run uses when it selects none.
func (c *Coordinator) DefaultProvider() string { return c.defaultProvider }
