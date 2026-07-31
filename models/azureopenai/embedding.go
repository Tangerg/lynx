package azureopenai

import (
	"errors"

	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/models/openai"
)

type EmbeddingModelConfig struct {
	APIKey         string
	BaseURL        string
	DefaultOptions embedding.Options
	RequestOptions []option.RequestOption
}

func (c EmbeddingModelConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("azureopenai: APIKey is required")
	}
	if c.DefaultOptions.Model == "" {
		return errors.New("azureopenai: DefaultOptions.Model is required")
	}
	if _, err := c.DefaultOptions.Merged(); err != nil {
		return err
	}
	return nil
}

// NewEmbeddingModel returns an [openai.EmbeddingModel] pointed at Azure
// OpenAI's v1 /embeddings endpoint. [embedding.Options].Model is the Azure
// deployment id.
func NewEmbeddingModel(cfg EmbeddingModelConfig) (*openai.EmbeddingModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	reqOpts, err := buildRequestOptions(cfg.BaseURL, cfg.RequestOptions)
	if err != nil {
		return nil, err
	}
	return openai.NewEmbeddingModel(openai.EmbeddingModelConfig{
		APIKey:         cfg.APIKey,
		DefaultOptions: cfg.DefaultOptions,
		RequestOptions: reqOpts,
	})
}
