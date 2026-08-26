package mistral

import (
	"cmp"
	"errors"
	"net/http"

	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/models/protocol/openai"
)

type EmbeddingModelConfig struct {
	APIKey         string
	DefaultOptions embedding.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c EmbeddingModelConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("mistral: APIKey is required")
	}
	if c.DefaultOptions.Model == "" {
		return errors.New("mistral: DefaultOptions.Model is required")
	}
	if _, err := c.DefaultOptions.Merged(); err != nil {
		return err
	}
	return nil
}

var _ embedding.Model = (*EmbeddingModel)(nil)

type EmbeddingModel = openai.EmbeddingModel

// NewEmbeddingModel returns a Mistral-compatible embedding model.
func NewEmbeddingModel(cfg EmbeddingModelConfig) (*EmbeddingModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return openai.NewEmbeddingModel(openai.EmbeddingModelConfig{
		Provider:       "mistral",
		APIKey:         cfg.APIKey,
		DefaultOptions: cfg.DefaultOptions,
		BaseURL:        cmp.Or(cfg.BaseURL, DefaultBaseURL),
		HTTPClient:     cfg.HTTPClient,
	})
}
