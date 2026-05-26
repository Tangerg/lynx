package mistral

import (
	"cmp"
	"errors"

	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/embedding"
	"github.com/Tangerg/lynx/models/openai"
)

type EmbeddingModelConfig struct {
	APIKey         model.APIKey
	DefaultOptions *embedding.Options
	BaseURL        string

	// RequestOptions reach the underlying openai-go client; use
	// [option.WithHTTPClient] here to customize the HTTP transport.
	RequestOptions []option.RequestOption
}

func (c *EmbeddingModelConfig) validate() error {
	if c == nil {
		return errors.New("mistral: config must not be nil")
	}
	if c.APIKey == nil {
		return errors.New("mistral: APIKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("mistral: DefaultOptions is required")
	}
	return nil
}

// NewEmbeddingModel returns an openai-backed [embedding.Model] pointed
// at Mistral's /embeddings endpoint (OpenAI-compatible shape). Models:
// "mistral-embed", "codestral-embed".
func NewEmbeddingModel(cfg *EmbeddingModelConfig) (*openai.EmbeddingModel, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	baseURL := cmp.Or(cfg.BaseURL, DefaultBaseURL)
	reqOpts := append([]option.RequestOption{option.WithBaseURL(baseURL)}, cfg.RequestOptions...)
	return openai.NewEmbeddingModel(&openai.EmbeddingModelConfig{
		APIKey:         cfg.APIKey,
		DefaultOptions: cfg.DefaultOptions,
		RequestOptions: reqOpts,
		Metadata:       &embedding.ModelMetadata{Provider: Provider},
	})
}
