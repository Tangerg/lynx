package bootstrap

import (
	"context"

	"github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/llm"
)

// providerCatalog projects the infra provider adapter table into domain metadata
// for the capabilities coordinator. Only the composition root reads the infra
// catalog, so this projection lives here rather than in the application ring.
type providerCatalog struct{}

func (providerCatalog) Supported() []provider.Metadata {
	supported := llm.SupportedProviders()
	out := make([]provider.Metadata, 0, len(supported))
	for _, p := range supported {
		out = append(out, providerMetadata(p))
	}
	return out
}

func (providerCatalog) Metadata(id string) (provider.Metadata, bool) {
	if !llm.Provider(id).IsSupported() {
		return provider.Metadata{}, false
	}
	return providerMetadata(llm.Provider(id)), true
}

func providerMetadata(p llm.Provider) provider.Metadata {
	return provider.Metadata{
		ID:                    string(p),
		RequiresBaseURL:       p.RequiresBaseURL(),
		EmbeddingCapable:      p.EmbeddingCapable(),
		DefaultEmbeddingModel: p.DefaultEmbeddingModel(),
	}
}

// providerProber validates a provider's credentials by building its default-model
// client and issuing one minimal (max_tokens=1) request — the cheapest call that
// proves the key + endpoint work. Lives here because the composition root owns
// client construction against the infra provider adapters. Returns the provider
// error verbatim so the caller can surface it inline.
type providerProber struct{}

func (providerProber) Probe(ctx context.Context, entry provider.Provider) error {
	client, err := llm.BuildClient(llm.ClientSpec{
		Provider: llm.Provider(entry.ID),
		Model:    llm.Provider(entry.ID).DefaultModel(),
		APIKey:   entry.APIKey,
		BaseURL:  entry.BaseURL,
	})
	if err != nil {
		return err
	}
	maxTokens := int64(1)
	_, err = client.Call(ctx, &chat.Request{
		Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart("ping"))},
		Options:  chat.Options{MaxTokens: &maxTokens},
	})
	return err
}
