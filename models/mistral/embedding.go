package mistral

import (
	"cmp"
	"context"
	"errors"

	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/models/internal/protocol/openai"
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

var _ embedding.Model = (*EmbeddingModel)(nil)

type EmbeddingModel struct{ protocol *openai.EmbeddingModel }

// NewEmbeddingModel returns a Mistral-compatible embedding model.
func NewEmbeddingModel(cfg EmbeddingModelConfig) (*EmbeddingModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	baseURL := cmp.Or(cfg.BaseURL, DefaultBaseURL)
	reqOpts := append([]option.RequestOption{option.WithBaseURL(baseURL)}, cfg.RequestOptions...)
	protocol, err := openai.NewEmbeddingModel(openai.EmbeddingModelConfig{
		Provider:       "mistral",
		APIKey:         cfg.APIKey,
		DefaultOptions: cfg.DefaultOptions,
		RequestOptions: reqOpts,
	})
	if err != nil {
		return nil, err
	}
	return &EmbeddingModel{protocol: protocol}, nil
}

func (m *EmbeddingModel) Call(ctx context.Context, req *embedding.Request) (*embedding.Response, error) {
	if m == nil || m.protocol == nil {
		return nil, errors.New("mistral: nil EmbeddingModel")
	}
	return m.protocol.Call(ctx, req)
}
