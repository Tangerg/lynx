package mistral

import (
	"cmp"
	"errors"

	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/models/openai"
)

type EmbeddingModelConfig struct {
	APIKey         string
	DefaultOptions embedding.Options
	BaseURL        string

	// RequestOptions reach the underlying openai-go client; use
	// [option.WithHTTPClient] here to customize the HTTP transport.
	RequestOptions []option.RequestOption
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

// NewEmbeddingModel returns an openai-backed [embedding.Model] pointed
// at Mistral's /embeddings endpoint (OpenAI-compatible shape). Models:
// "mistral-embed", "codestral-embed".
func NewEmbeddingModel(cfg EmbeddingModelConfig) (*openai.EmbeddingModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	baseURL := cmp.Or(cfg.BaseURL, DefaultBaseURL)
	reqOpts := append([]option.RequestOption{option.WithBaseURL(baseURL)}, cfg.RequestOptions...)
	return openai.NewEmbeddingModel(openai.EmbeddingModelConfig{
		APIKey:         cfg.APIKey,
		DefaultOptions: cfg.DefaultOptions,
		RequestOptions: reqOpts,
	})
}
