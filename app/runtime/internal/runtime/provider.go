package runtime

import (
	"context"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/llm"
)

// Providers returns the provider registry — the runtime-mutable set of
// providers + credentials that providers.list / configure / test operate on.
// Always non-nil.
func (r *Runtime) Providers() provider.Service { return r.providers }

// ListRegisteredProviders returns the runtime-mutable provider registry.
func (r *Runtime) ListRegisteredProviders(ctx context.Context) ([]provider.Provider, error) {
	return r.providers.List(ctx)
}

// GetRegisteredProvider returns one provider registry entry by provider id.
func (r *Runtime) GetRegisteredProvider(ctx context.Context, id string) (provider.Provider, bool, error) {
	return r.providers.Get(ctx, id)
}

// ConfigureProvider upserts a provider's credentials into the registry.
func (r *Runtime) ConfigureProvider(ctx context.Context, entry provider.Provider) error {
	return r.providers.Configure(ctx, entry)
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
