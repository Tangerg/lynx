package azureopenai

import (
	"github.com/Tangerg/scope/core/image"
	"github.com/Tangerg/scope/models/protocol/openai"
)

type ImageModelConfig struct {
	Config
	DefaultOptions image.Options
}

func (i ImageModelConfig) resolve() (endpointConfig, error) {
	return resolveModelConfig(i.Config, i.DefaultOptions.Model, i.DefaultOptions.Validate)
}

func (i ImageModelConfig) Validate() error {
	_, err := i.resolve()
	return err
}

var _ image.Model = (*ImageModel)(nil)

type ImageModel = openai.ImageModel

func NewImageModel(config ImageModelConfig) (*ImageModel, error) {
	endpoint, err := config.resolve()
	if err != nil {
		return nil, err
	}
	return openai.NewImageModel(openai.ImageModelConfig{
		Provider:       protocolProvider,
		APIKey:         endpoint.apiKey,
		DefaultOptions: config.DefaultOptions,
		BaseURL:        endpoint.baseURL,
		HTTPClient:     endpoint.httpClient,
	})
}
