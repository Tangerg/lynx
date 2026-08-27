package models

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/scope/app/runtime/internal/application/invalidation"
	"github.com/Tangerg/scope/app/runtime/internal/application/secrets"
	"github.com/Tangerg/scope/app/runtime/internal/domain/provider"
)

// ProviderSummary is the application result for provider discovery and
// configuration. It intentionally carries only the redacted credential view.
type ProviderSummary struct {
	ID                    string
	BaseURL               string
	APIKeyMasked          string
	KeySource             provider.KeySource
	RequiresBaseURL       bool
	EmbeddingCapable      bool
	DefaultEmbeddingModel string
}

// UpdateProviderCommand is an atomic partial change to provider configuration.
// Nil fields preserve the stored value; a non-nil empty string clears it.
type UpdateProviderCommand struct {
	ID      string
	APIKey  *string
	BaseURL *string
}

// ProviderTestOutcome is the complete client-relevant result of a live
// credential probe. Unsupported provider remains a command error; all other
// operational details stay in observability rather than becoming arbitrary
// caller-visible error text.
type ProviderTestOutcome string

const (
	ProviderTestSucceeded     ProviderTestOutcome = "succeeded"
	ProviderTestNotConfigured ProviderTestOutcome = "not_configured"
	ProviderTestFailed        ProviderTestOutcome = "failed"
)

// ListProviders returns the supported-provider set annotated with its current
// configuration. Registry-only unknown providers are intentionally omitted.
func (c *Coordinator) ListProviders(ctx context.Context) ([]ProviderSummary, error) {
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
	out := make([]ProviderSummary, 0, len(metadata))
	for _, meta := range metadata {
		out = append(out, providerSummary(meta, byID[meta.ID]))
	}
	return out, nil
}

// UpdateProvider validates and persists one supported provider, returning
// its redacted stored result.
func (c *Coordinator) UpdateProvider(ctx context.Context, cmd UpdateProviderCommand) (ProviderSummary, error) {
	meta, err := c.supportedProvider(cmd.ID)
	if err != nil {
		return ProviderSummary{}, err
	}
	if c.providers == nil {
		return ProviderSummary{}, errors.New("models: provider registry is unavailable")
	}
	patch := provider.Patch{APIKey: cmd.APIKey, BaseURL: cmd.BaseURL}
	if patch.Empty() {
		return ProviderSummary{}, fmt.Errorf("%w: provider %q has no changes", ErrProviderUpdateRequired, cmd.ID)
	}
	if meta.RequiresBaseURL {
		if cmd.BaseURL != nil {
			if *cmd.BaseURL == "" {
				return ProviderSummary{}, fmt.Errorf("%w: provider %q", ErrProviderBaseURLRequired, cmd.ID)
			}
		} else {
			existing, _, getErr := c.providers.Get(ctx, cmd.ID)
			if getErr != nil {
				return ProviderSummary{}, getErr
			}
			if existing.BaseURL == "" {
				return ProviderSummary{}, fmt.Errorf("%w: provider %q", ErrProviderBaseURLRequired, cmd.ID)
			}
		}
	}
	entry, err := c.providers.Update(ctx, cmd.ID, patch)
	if err != nil {
		return ProviderSummary{}, err
	}
	c.invalidations.Notify(invalidation.Notice{Resource: invalidation.Models})
	return providerSummary(meta, entry), nil
}

// TestProvider checks that a supported, configured provider accepts a minimal
// request. Its result is deliberately a stable use-case outcome; integration
// diagnostics never become caller-visible data.
func (c *Coordinator) TestProvider(ctx context.Context, id string) (ProviderTestOutcome, error) {
	_, entry, err := c.configuredProvider(ctx, id)
	if err != nil {
		if errors.Is(err, ErrProviderUnsupported) {
			return "", err
		}
		if errors.Is(err, ErrProviderUnconfigured) {
			return ProviderTestNotConfigured, nil
		}
		trace.SpanFromContext(ctx).RecordError(err)
		return ProviderTestFailed, nil
	}
	if c.prober == nil {
		trace.SpanFromContext(ctx).RecordError(errors.New("models: provider probe is unavailable"))
		return ProviderTestFailed, nil
	}
	if err := c.prober.Probe(ctx, entry); err != nil {
		trace.SpanFromContext(ctx).RecordError(err)
		return ProviderTestFailed, nil
	}
	return ProviderTestSucceeded, nil
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

func (c *Coordinator) supportedProviders() []ProviderMetadata {
	if c.catalog == nil {
		return nil
	}
	return c.catalog.Supported()
}

func (c *Coordinator) providerMetadata(id string) (ProviderMetadata, bool) {
	if c.catalog == nil {
		return ProviderMetadata{}, false
	}
	return c.catalog.Metadata(id)
}

func (c *Coordinator) supportedProvider(id string) (ProviderMetadata, error) {
	meta, ok := c.providerMetadata(id)
	if !ok {
		return ProviderMetadata{}, fmt.Errorf("%w: provider %q", ErrProviderUnsupported, id)
	}
	return meta, nil
}

func (c *Coordinator) configuredProvider(ctx context.Context, id string) (ProviderMetadata, provider.Provider, error) {
	meta, err := c.supportedProvider(id)
	if err != nil {
		return ProviderMetadata{}, provider.Provider{}, err
	}
	if c.providers == nil {
		return ProviderMetadata{}, provider.Provider{}, errors.New("models: provider registry is unavailable")
	}
	entry, ok, err := c.providers.Get(ctx, id)
	if err != nil {
		return ProviderMetadata{}, provider.Provider{}, err
	}
	if !ok || !entry.Enabled() {
		return ProviderMetadata{}, provider.Provider{}, fmt.Errorf("%w: provider %q", ErrProviderUnconfigured, id)
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

func providerSummary(meta ProviderMetadata, entry provider.Provider) ProviderSummary {
	return ProviderSummary{
		ID:                    meta.ID,
		BaseURL:               entry.BaseURL,
		APIKeyMasked:          secrets.Mask(entry.APIKey),
		KeySource:             entry.KeySource,
		RequiresBaseURL:       meta.RequiresBaseURL,
		EmbeddingCapable:      meta.EmbeddingCapable,
		DefaultEmbeddingModel: meta.DefaultEmbeddingModel,
	}
}
