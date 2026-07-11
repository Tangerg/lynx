package capabilities

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
)

// ListRegisteredProviders returns the runtime-mutable provider registry.
func (c *Coordinator) ListRegisteredProviders(ctx context.Context) ([]provider.Provider, error) {
	return c.providers.List(ctx)
}

// RegisteredProvider returns one provider registry entry by provider id.
func (c *Coordinator) RegisteredProvider(ctx context.Context, id string) (provider.Provider, bool, error) {
	return c.providers.Get(ctx, id)
}

// ConfigureProvider upserts a provider's credentials into the registry.
func (c *Coordinator) ConfigureProvider(ctx context.Context, entry provider.Provider) error {
	return c.providers.Configure(ctx, entry)
}

// SupportedProviders returns the static provider reference data this build can
// serve. Configuration state lives in the registry; this is the composition
// root's projection of the infra adapter catalog into domain values. A nil
// catalog (a coordinator wired without one) serves no providers.
func (c *Coordinator) SupportedProviders() []provider.Metadata {
	if c.catalog == nil {
		return nil
	}
	return c.catalog.Supported()
}

// ProviderMetadata returns the static adapter metadata for id.
func (c *Coordinator) ProviderMetadata(id string) (provider.Metadata, bool) {
	if c.catalog == nil {
		return provider.Metadata{}, false
	}
	return c.catalog.Metadata(id)
}

// ProbeProvider validates a provider's credentials with one minimal live call
// (providers.test), returning the provider error verbatim so the caller can
// surface it inline.
func (c *Coordinator) ProbeProvider(ctx context.Context, entry provider.Provider) error {
	return c.prober.Probe(ctx, entry)
}
