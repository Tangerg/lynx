package azureopenai

import (
	"errors"

	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/embedding"
	"github.com/Tangerg/lynx/models/openai"
)

type EmbeddingModelConfig struct {
	ApiKey         model.ApiKey
	Endpoint       string
	APIVersion     string
	DefaultOptions *embedding.Options
	RequestOptions []option.RequestOption
}

func (c *EmbeddingModelConfig) validate() error {
	if c == nil {
		return errors.New("azureopenai: config must not be nil")
	}
	if c.Endpoint == "" {
		return errors.New("azureopenai: Endpoint is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("azureopenai: DefaultOptions is required")
	}
	return nil
}

// NewEmbeddingModel returns an [openai.EmbeddingModel] pointed at
// Azure OpenAI. [embedding.Options].Model is the Azure deployment id.
func NewEmbeddingModel(cfg *EmbeddingModelConfig) (*openai.EmbeddingModel, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	apiKey, reqOpts := buildAzureRequestOptions(cfg.ApiKey, cfg.Endpoint, cfg.APIVersion, cfg.RequestOptions)
	return openai.NewEmbeddingModel(&openai.EmbeddingModelConfig{
		ApiKey:         apiKey,
		DefaultOptions: cfg.DefaultOptions,
		RequestOptions: reqOpts,
		Metadata:       &embedding.ModelMetadata{Provider: Provider},
	})
}
