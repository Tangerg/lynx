package alibaba

import (
	"cmp"
	"errors"

	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/model"
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

func (c EmbeddingModelConfig) Validate() error {
	if c.APIKey == nil {
		return errors.New("alibaba: APIKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("alibaba: DefaultOptions is required")
	}
	return nil
}

// NewEmbeddingModel returns an openai-backed embedding model pointed
// at DashScope's compatible-mode /embeddings. text-embedding-v3 and
// text-embedding-v4 both accept the OpenAI-shaped dimensions param
// via [embedding.Options.Dimensions].
func NewEmbeddingModel(cfg EmbeddingModelConfig) (*openai.EmbeddingModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	baseURL := cmp.Or(cfg.BaseURL, BaseURLChina)
	reqOpts := append([]option.RequestOption{option.WithBaseURL(baseURL)}, cfg.RequestOptions...)
	return openai.NewEmbeddingModel(openai.EmbeddingModelConfig{
		APIKey:         cfg.APIKey,
		DefaultOptions: cfg.DefaultOptions,
		RequestOptions: reqOpts,
		Metadata:       &embedding.ModelMetadata{Provider: Provider},
	})
}
