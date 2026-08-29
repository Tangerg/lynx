package azureopenai

import (
	"github.com/Tangerg/scope/core/embedding"
	"github.com/Tangerg/scope/models/protocol/openai"
)

type EmbeddingModelConfig struct {
	Config
	DefaultOptions embedding.Options
}

func (e EmbeddingModelConfig) resolve() (endpointConfig, error) {
	return resolveModelConfig(e.Config, e.DefaultOptions.Model, e.DefaultOptions.Validate)
}

func (e EmbeddingModelConfig) Validate() error {
	_, err := e.resolve()
	return err
}

var _ embedding.Model = (*EmbeddingModel)(nil)

type EmbeddingModel = openai.EmbeddingModel

func NewEmbeddingModel(config EmbeddingModelConfig) (*EmbeddingModel, error) {
	endpoint, err := config.resolve()
	if err != nil {
		return nil, err
	}
	return openai.NewEmbeddingModel(openai.EmbeddingModelConfig{
		Provider:       protocolProvider,
		APIKey:         endpoint.apiKey,
		DefaultOptions: config.DefaultOptions,
		BaseURL:        endpoint.baseURL,
		HTTPClient:     endpoint.httpClient,
	})
}
