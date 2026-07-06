package server

import (
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
)

func providerListWire(configured []provider.Provider, supported []provider.Metadata) []protocol.Provider {
	byID := make(map[string]provider.Provider, len(configured))
	for _, p := range configured {
		byID[p.ID] = p
	}
	out := make([]protocol.Provider, 0, len(supported))
	for _, meta := range supported {
		out = append(out, providerToWire(meta, byID[meta.ID]))
	}
	return out
}

func providerToWire(meta provider.Metadata, entry provider.Provider) protocol.Provider {
	return protocol.Provider{
		ID:                    meta.ID,
		BaseURL:               entry.BaseURL,
		APIKeyMasked:          entry.MaskedAPIKey(),
		KeySource:             string(entry.KeySource),
		RequiresBaseURL:       meta.RequiresBaseURL,
		EmbeddingCapable:      meta.EmbeddingCapable,
		DefaultEmbeddingModel: meta.DefaultEmbeddingModel,
	}
}
