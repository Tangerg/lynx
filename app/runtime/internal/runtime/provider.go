package runtime

import (
	"context"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/llm"
)

type providerRegistryList interface {
	List(ctx context.Context) ([]provider.Provider, error)
}

type providerRegistryRead interface {
	Get(ctx context.Context, id string) (provider.Provider, bool, error)
}

type providerRegistryConfigure interface {
	Configure(ctx context.Context, entry provider.Provider) error
}

// ListRegisteredProviders returns the runtime-mutable provider registry.
func (r *Runtime) ListRegisteredProviders(ctx context.Context) ([]provider.Provider, error) {
	return r.providerRegistryList.List(ctx)
}

// RegisteredProvider returns one provider registry entry by provider id.
func (r *Runtime) RegisteredProvider(ctx context.Context, id string) (provider.Provider, bool, error) {
	return r.providerRegistryRead.Get(ctx, id)
}

// ConfigureProvider upserts a provider's credentials into the registry.
func (r *Runtime) ConfigureProvider(ctx context.Context, entry provider.Provider) error {
	return r.providerRegistryConfigure.Configure(ctx, entry)
}

// SupportedProviders returns the provider reference data this runtime build can
// serve. Configuration state lives in the registry; this is the static adapter
// catalog projected into domain values for delivery layers.
func (r *Runtime) SupportedProviders() []provider.Metadata {
	supported := llm.SupportedProviders()
	out := make([]provider.Metadata, 0, len(supported))
	for _, p := range supported {
		out = append(out, provider.Metadata{
			ID:                    string(p),
			RequiresBaseURL:       llm.RequiresBaseURL(p),
			EmbeddingCapable:      llm.EmbeddingCapable(p),
			DefaultEmbeddingModel: llm.DefaultEmbeddingModel(p),
		})
	}
	return out
}

// ProviderMetadata returns the static adapter metadata for id.
func (r *Runtime) ProviderMetadata(id string) (provider.Metadata, bool) {
	if !llm.IsSupported(llm.Provider(id)) {
		return provider.Metadata{}, false
	}
	p := llm.Provider(id)
	return provider.Metadata{
		ID:                    id,
		RequiresBaseURL:       llm.RequiresBaseURL(p),
		EmbeddingCapable:      llm.EmbeddingCapable(p),
		DefaultEmbeddingModel: llm.DefaultEmbeddingModel(p),
	}, true
}

// ProbeProvider validates a provider's credentials by building its
// default-model client and issuing one minimal (max_tokens=1) request — the
// cheapest call that proves the key + endpoint work. Backs providers.test.
// Lives here, not in the protocol layer, because the runtime owns client
// construction. Returns the provider error verbatim so the caller can surface
// it inline.
func (r *Runtime) ProbeProvider(ctx context.Context, entry provider.Provider) error {
	client, err := llm.BuildClient(llm.ClientSpec{
		Provider: llm.Provider(entry.ID),
		Model:    llm.DefaultModel(llm.Provider(entry.ID)),
		APIKey:   entry.APIKey,
		BaseURL:  entry.BaseURL,
	})
	if err != nil {
		return err
	}
	maxTokens := int64(1)
	_, err = client.Chat().
		WithOptions(&chat.Options{MaxTokens: &maxTokens}).
		WithUserPrompt("ping").
		Call().
		Response(ctx)
	return err
}
