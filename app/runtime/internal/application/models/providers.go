package models

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
)

// ProviderInfo is the application result for provider discovery and
// configuration. It intentionally carries only the redacted credential view.
type ProviderInfo struct {
	ID                    string
	BaseURL               string
	APIKeyMasked          string
	KeySource             provider.KeySource
	RequiresBaseURL       bool
	EmbeddingCapable      bool
	DefaultEmbeddingModel string
}

// ConfigureProviderCommand is the editable provider configuration.
type ConfigureProviderCommand struct {
	ID      string
	APIKey  string
	BaseURL string
}

// ListProviders returns the supported-provider set annotated with its current
// configuration. Registry-only unknown providers are intentionally omitted.
func (c *Coordinator) ListProviders(ctx context.Context) ([]ProviderInfo, error) {
	if c.providers == nil {
		return nil, errors.New("models: provider registry is unavailable")
	}
	entries, err := c.providers.List(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]provider.Provider, len(entries))
	for _, entry := range entries {
		byID[entry.ID] = entry
	}
	metadata := c.supportedProviders()
	out := make([]ProviderInfo, 0, len(metadata))
	for _, meta := range metadata {
		out = append(out, providerInfo(meta, byID[meta.ID]))
	}
	return out, nil
}

// ConfigureProvider validates and persists one supported provider, returning
// its redacted stored result.
func (c *Coordinator) ConfigureProvider(ctx context.Context, cmd ConfigureProviderCommand) (ProviderInfo, error) {
	meta, err := c.supportedProvider(cmd.ID)
	if err != nil {
		return ProviderInfo{}, err
	}
	if meta.RequiresBaseURL && cmd.BaseURL == "" {
		return ProviderInfo{}, fmt.Errorf("%w: provider %q", ErrProviderBaseURLRequired, cmd.ID)
	}
	if c.providers == nil {
		return ProviderInfo{}, errors.New("models: provider registry is unavailable")
	}
	if err := c.providers.Configure(ctx, provider.Provider{ID: cmd.ID, APIKey: cmd.APIKey, BaseURL: cmd.BaseURL}); err != nil {
		return ProviderInfo{}, err
	}
	entry, ok, err := c.providers.Get(ctx, cmd.ID)
	if err != nil {
		return ProviderInfo{}, err
	}
	if !ok {
		return ProviderInfo{}, fmt.Errorf("%w: provider %q", ErrProviderReadBack, cmd.ID)
	}
	return providerInfo(meta, entry), nil
}

// TestProvider checks that a supported, configured provider accepts a minimal
// request. Probe failures remain operational errors so callers can present the
// provider's diagnostic without reimplementing configuration policy.
func (c *Coordinator) TestProvider(ctx context.Context, id string) error {
	_, entry, err := c.configuredProvider(ctx, id)
	if err != nil {
		return err
	}
	if c.prober == nil {
		return errors.New("models: provider probe is unavailable")
	}
	return c.prober.Probe(ctx, entry)
}

// ListModels applies the model-discovery policy. Providers with endpoint-owned
// model sets prefer a successful non-empty remote list; every other outcome
// falls back to the static catalog, so restart behavior never depends on an
// in-memory probe result.
func (c *Coordinator) ListModels(ctx context.Context, providerID string) []Model {
	meta, found := c.providerMetadata(providerID)
	if found && meta.ProbeModels {
		if ids, err := c.listRemoteModels(ctx, providerID); err == nil && len(ids) > 0 {
			out := make([]Model, 0, len(ids))
			for _, id := range ids {
				if model, ok := c.lookupModel(providerID, id); ok {
					out = append(out, model)
					continue
				}
				out = append(out, Model{ID: id, Provider: providerID})
			}
			return out
		}
	}
	return c.catalogModels(providerID)
}

func (c *Coordinator) supportedProviders() []provider.Metadata {
	if c.catalog == nil {
		return nil
	}
	return c.catalog.Supported()
}

func (c *Coordinator) providerMetadata(id string) (provider.Metadata, bool) {
	if c.catalog == nil {
		return provider.Metadata{}, false
	}
	return c.catalog.Metadata(id)
}

func (c *Coordinator) supportedProvider(id string) (provider.Metadata, error) {
	meta, ok := c.providerMetadata(id)
	if !ok {
		return provider.Metadata{}, fmt.Errorf("%w: provider %q", ErrProviderUnsupported, id)
	}
	return meta, nil
}

func (c *Coordinator) configuredProvider(ctx context.Context, id string) (provider.Metadata, provider.Provider, error) {
	meta, err := c.supportedProvider(id)
	if err != nil {
		return provider.Metadata{}, provider.Provider{}, err
	}
	if c.providers == nil {
		return provider.Metadata{}, provider.Provider{}, errors.New("models: provider registry is unavailable")
	}
	entry, ok, err := c.providers.Get(ctx, id)
	if err != nil {
		return provider.Metadata{}, provider.Provider{}, err
	}
	if !ok || !entry.Enabled() {
		return provider.Metadata{}, provider.Provider{}, fmt.Errorf("%w: provider %q", ErrProviderUnconfigured, id)
	}
	return meta, entry, nil
}

func (c *Coordinator) listRemoteModels(ctx context.Context, providerID string) ([]string, error) {
	if c.lister == nil {
		return nil, nil
	}
	entry := provider.Provider{ID: providerID}
	if c.providers != nil {
		configured, ok, err := c.providers.Get(ctx, providerID)
		if err != nil {
			return nil, err
		}
		if ok {
			entry = configured
		}
	}
	return c.lister.ListModels(ctx, entry)
}

func (c *Coordinator) catalogModels(providerID string) []Model {
	if c.catalog == nil {
		return nil
	}
	return c.catalog.Models(providerID)
}

func (c *Coordinator) lookupModel(providerID, modelID string) (Model, bool) {
	if c.catalog == nil {
		return Model{}, false
	}
	return c.catalog.LookupModel(providerID, modelID)
}

func providerInfo(meta provider.Metadata, entry provider.Provider) ProviderInfo {
	return ProviderInfo{
		ID:                    meta.ID,
		BaseURL:               entry.BaseURL,
		APIKeyMasked:          entry.MaskedAPIKey(),
		KeySource:             entry.KeySource,
		RequiresBaseURL:       meta.RequiresBaseURL,
		EmbeddingCapable:      meta.EmbeddingCapable,
		DefaultEmbeddingModel: meta.DefaultEmbeddingModel,
	}
}
