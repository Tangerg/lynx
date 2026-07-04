package server

import (
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/llm"
)

func providerListWire(configured []provider.Provider) []protocol.Provider {
	byID := make(map[string]provider.Provider, len(configured))
	for _, p := range configured {
		byID[p.ID] = p
	}
	supported := llm.SupportedProviders()
	out := make([]protocol.Provider, 0, len(supported))
	for _, sp := range supported {
		id := string(sp)
		out = append(out, providerToWire(id, byID[id]))
	}
	return out
}

func providerToWire(id string, entry provider.Provider) protocol.Provider {
	return protocol.Provider{
		ID:                    id,
		BaseURL:               entry.BaseURL,
		APIKeyMasked:          entry.MaskedAPIKey(),
		KeySource:             string(entry.KeySource),
		RequiresBaseURL:       llm.RequiresBaseURL(llm.Provider(id)),
		EmbeddingCapable:      llm.EmbeddingCapable(llm.Provider(id)),
		DefaultEmbeddingModel: llm.DefaultEmbeddingModel(llm.Provider(id)),
	}
}

func isSupportedProvider(id string) bool {
	return llm.IsSupported(llm.Provider(id))
}
