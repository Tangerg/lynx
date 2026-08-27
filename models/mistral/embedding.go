package mistral

import (
	"cmp"
	"errors"
	"net/http"

	"github.com/Tangerg/scope/core/embedding"
	"github.com/Tangerg/scope/models/protocol/openai"
)

type EmbeddingModelConfig struct {
	APIKey         string
	DefaultOptions embedding.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (e EmbeddingModelConfig) Validate() error {
	if e.APIKey == "" {
		return errors.New("mistral: APIKey is required")
	}
	if e.DefaultOptions.Model == "" {
		return errors.New("mistral: DefaultOptions.Model is required")
	}
	if err := e.DefaultOptions.Validate(); err != nil {
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
