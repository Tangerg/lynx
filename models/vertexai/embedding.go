package vertexai

import (
	"context"
	"errors"
	"net/http"

	"google.golang.org/genai"

	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/models/internal/protocol/google"
)

type EmbeddingModelConfig struct {
	Project        string
	Location       string
	DefaultOptions embedding.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c EmbeddingModelConfig) Validate() error {
	if c.Project == "" {
		return errors.New("vertexai: Project is required")
	}
	if c.Location == "" {
		return errors.New("vertexai: Location is required")
	}
	if c.DefaultOptions.Model == "" {
		return errors.New("vertexai: DefaultOptions.Model is required")
	}
	if _, err := c.DefaultOptions.Merged(); err != nil {
		return err
	}
	return nil
}

var _ embedding.Model = (*EmbeddingModel)(nil)

type EmbeddingModel struct{ protocol *google.EmbeddingModel }

// NewEmbeddingModel returns a Vertex AI embedding model.
func NewEmbeddingModel(cfg EmbeddingModelConfig) (*EmbeddingModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	protocol, err := google.NewEmbeddingModel(google.EmbeddingModelConfig{
		Provider:       "vertexai",
		Backend:        genai.BackendVertexAI,
		Project:        cfg.Project,
		Location:       cfg.Location,
		DefaultOptions: cfg.DefaultOptions,
		BaseURL:        cfg.BaseURL,
		HTTPClient:     cfg.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return &EmbeddingModel{protocol: protocol}, nil
}

func (m *EmbeddingModel) Call(ctx context.Context, req *embedding.Request) (*embedding.Response, error) {
	if m == nil || m.protocol == nil {
		return nil, errors.New("vertexai: nil EmbeddingModel")
	}
	return m.protocol.Call(ctx, req)
}
