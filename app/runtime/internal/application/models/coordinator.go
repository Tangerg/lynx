// Package models is the application coordinator for provider + model
// configuration: the runtime-mutable provider registry (credentials), the static
// provider catalog + credential prober, and the utility / embedding model roles.
// It is a thin use-case layer over the domain provider registry and focused
// ports for model validation, catalog lookup, and credential probing.
package models

import (
	"context"
	"errors"
	"sync"

	"github.com/Tangerg/lynx/app/runtime/internal/application/invalidation"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
)

// ProviderCatalog supplies static provider and model reference data. The
// coordinator owns the use-case policy that consumes the projection.
type ProviderCatalog interface {
	Supported() []ProviderMetadata
	Metadata(id string) (ProviderMetadata, bool)
	Models(providerID string) []Model
	LookupModel(providerID, modelID string) (Model, bool)
}

var (
	// ErrProviderUnsupported reports a provider id with no runtime implementation.
	ErrProviderUnsupported = errors.New("models: provider is unsupported")
	// ErrProviderBaseURLRequired reports a provider that cannot be configured
	// without its endpoint.
	ErrProviderBaseURLRequired = errors.New("models: provider base URL is required")
	// ErrProviderUnconfigured reports a supported provider with no usable key.
	ErrProviderUnconfigured = errors.New("models: provider is not configured")
	// ErrProviderUpdateRequired reports a provider update with no changes.
	ErrProviderUpdateRequired = errors.New("models: provider update has no changes")
	// ErrEmbeddingUnsupported reports a provider with no embedding implementation.
	ErrEmbeddingUnsupported = errors.New("models: provider does not support embeddings")
)

// ProviderProber validates a provider's credentials with one minimal live call
// during a provider probe.
type ProviderProber interface {
	Probe(ctx context.Context, entry provider.Provider) error
}

// ProviderModelLister discovers a provider's available models by probing its
// live endpoint — used for local / bring-your-own-endpoint providers whose model
// set is not in the static catalog (dynamic discovery from an Ollama daemon or a compatible
// passthrough). A nil lister disables live discovery (static catalog only).
type ProviderModelLister interface {
	ListModels(ctx context.Context, entry provider.Provider) ([]string, error)
}

// ChatModelValidator verifies that a chat client can be built for
// (provider, model) without exposing the concrete client.
type ChatModelValidator interface {
	ValidateChatModel(ctx context.Context, providerID, model string) error
}

// EmbeddingModelValidator validates that an embedding client can be built for
// (provider, model) without exposing the concrete embedder.
type EmbeddingModelValidator interface {
	ValidateEmbeddingModel(ctx context.Context, providerID, model string) error
}

// UtilityRoleSaver persists the utility-model role across restarts. nil disables
// persistence (the role stays in-process only).
type UtilityRoleSaver interface {
	SaveUtilityRole(ctx context.Context, role modelref.Selection) error
}

// EmbeddingRoleSaver persists the embedding-model role across restarts. nil
// disables persistence.
type EmbeddingRoleSaver interface {
	SaveEmbeddingRole(ctx context.Context, role modelref.Selection) error
}

// Coordinator owns provider + model configuration. Any nil dependency disables
// the corresponding capability.
type Coordinator struct {
	providers ProviderRegistry
	catalog   ProviderCatalog
	prober    ProviderProber
	lister    ProviderModelLister

	// utility / embedding model roles: the live state shared with runtime
	// consumers, the resolver that validates a new role, and the saver
	// that persists it.
	utilityRoleState *RoleState
	utilityValidator ChatModelValidator
	utilityStore     UtilityRoleSaver
	utilityMu        sync.Mutex

	embeddingRoleState *RoleState
	embeddingValidator EmbeddingModelValidator
	embeddingStore     EmbeddingRoleSaver
	embeddingMu        sync.Mutex
	invalidations      invalidation.Publish
}

// Config bundles the Coordinator's dependencies.
type Config struct {
	Providers ProviderRegistry
	Catalog   ProviderCatalog
	Prober    ProviderProber
	Lister    ProviderModelLister

	UtilityRoleState *RoleState
	UtilityValidator ChatModelValidator
	UtilityStore     UtilityRoleSaver

	EmbeddingRoleState *RoleState
	EmbeddingValidator EmbeddingModelValidator
	EmbeddingStore     EmbeddingRoleSaver

	Invalidations invalidation.Publish
}

// New returns a models Coordinator over cfg.
func New(cfg Config) *Coordinator {
	if cfg.UtilityRoleState == nil {
		cfg.UtilityRoleState = NewRoleState(modelref.Selection{})
	}
	if cfg.EmbeddingRoleState == nil {
		cfg.EmbeddingRoleState = NewRoleState(modelref.Selection{})
	}
	return &Coordinator{
		providers:          cfg.Providers,
		catalog:            cfg.Catalog,
		prober:             cfg.Prober,
		lister:             cfg.Lister,
		utilityRoleState:   cfg.UtilityRoleState,
		utilityValidator:   cfg.UtilityValidator,
		utilityStore:       cfg.UtilityStore,
		embeddingRoleState: cfg.EmbeddingRoleState,
		embeddingValidator: cfg.EmbeddingValidator,
		embeddingStore:     cfg.EmbeddingStore,
		invalidations:      cfg.Invalidations,
	}
}
