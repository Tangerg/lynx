package server

import (
	modelapp "github.com/Tangerg/lynx/app/runtime/internal/application/models"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func providerToWire(provider modelapp.ProviderInfo) protocol.Provider {
	return protocol.Provider{
		ID:                    provider.ID,
		BaseURL:               provider.BaseURL,
		APIKeyMasked:          provider.APIKeyMasked,
		KeySource:             string(provider.KeySource),
		RequiresBaseURL:       provider.RequiresBaseURL,
		EmbeddingCapable:      provider.EmbeddingCapable,
		DefaultEmbeddingModel: provider.DefaultEmbeddingModel,
	}
}
