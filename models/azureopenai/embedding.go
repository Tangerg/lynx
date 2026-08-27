package azureopenai

import (
	"errors"
	"net/http"

	"github.com/Tangerg/scope/core/embedding"
	"github.com/Tangerg/scope/models/protocol/openai"
)

type EmbeddingModelConfig struct {
	APIKey         string
	BaseURL        string
	DefaultOptions embedding.Options
	HTTPClient     *http.Client
}

func (e EmbeddingModelConfig) Validate() error {
	if e.APIKey == "" {
		return errors.New("azureopenai: APIKey is required")
	}
	if e.DefaultOptions.Model == "" {
		return errors.New("azureopenai: DefaultOptions.Model is required")
	}
	if err := e.DefaultOptions.Validate(); err != nil {
		return err
	}
	return nil
}

var _ embedding.Model = (*EmbeddingModel)(nil)

type EmbeddingModel = openai.EmbeddingModel

// NewEmbeddingModel returns an Azure OpenAI embedding model. Model is the
// Azure deployment id.
func NewEmbeddingModel(cfg EmbeddingModelConfig) (*EmbeddingModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	baseURL, err := normalizeBaseURL(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	return openai.NewEmbeddingModel(openai.EmbeddingModelConfig{
		Provider:       "azureopenai",
		APIKey:         cfg.APIKey,
		DefaultOptions: cfg.DefaultOptions,
		BaseURL:        baseURL,
		HTTPClient:     cfg.HTTPClient,
	})
}
