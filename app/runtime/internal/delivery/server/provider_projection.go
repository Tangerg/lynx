package server

import (
	"fmt"

	modelapp "github.com/Tangerg/lynx/app/runtime/internal/application/models"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
)

func providerToWire(info modelapp.ProviderInfo) (protocol.Provider, error) {
	keySource, err := providerKeySourceWire(info.KeySource)
	if err != nil {
		return protocol.Provider{}, err
	}
	return protocol.Provider{
		ID:                    info.ID,
		BaseURL:               info.BaseURL,
		APIKeyMasked:          info.APIKeyMasked,
		KeySource:             keySource,
		RequiresBaseURL:       info.RequiresBaseURL,
		EmbeddingCapable:      info.EmbeddingCapable,
		DefaultEmbeddingModel: info.DefaultEmbeddingModel,
	}, nil
}

func providerKeySourceWire(source provider.KeySource) (protocol.ProviderKeySource, error) {
	switch source {
	case provider.KeyStored:
		return protocol.ProviderKeySourceStored, nil
	case provider.KeyEnv:
		return protocol.ProviderKeySourceEnv, nil
	case provider.KeyNone:
		return "", nil
	default:
		return "", fmt.Errorf("providers: unsupported key source %q", source)
	}
}
