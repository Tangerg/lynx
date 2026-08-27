package vertexai

import (
	"context"
	"errors"
	"net/http"

	"google.golang.org/genai"

	"github.com/Tangerg/scope/core/embedding"
	"github.com/Tangerg/scope/models/google/internal/protocol"
)

type EmbeddingModelConfig struct {
	Project        string
	Location       string
	DefaultOptions embedding.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (e EmbeddingModelConfig) Validate() error {
	if e.Project == "" {
		return errors.New("vertexai: Project is required")
	}
	if e.Location == "" {
		return errors.New("vertexai: Location is required")
	}
	if e.DefaultOptions.Model == "" {
		return errors.New("vertexai: DefaultOptions.Model is required")
	}
	if err := e.DefaultOptions.Validate(); err != nil {
		return err
	}
	return nil
}

var _ embedding.Model = (*EmbeddingModel)(nil)

type EmbeddingModel struct{ protocol *protocol.EmbeddingModel }

func NewEmbeddingModel(config EmbeddingModelConfig) (*EmbeddingModel, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	adapter, err := protocol.NewEmbeddingModel(protocol.EmbeddingModelConfig{
		Provider:       "vertexai",
		Backend:        genai.BackendVertexAI,
		Project:        config.Project,
		Location:       config.Location,
		DefaultOptions: config.DefaultOptions,
		BaseURL:        config.BaseURL,
		HTTPClient:     config.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return &EmbeddingModel{protocol: adapter}, nil
}

func (e *EmbeddingModel) Call(ctx context.Context, req *embedding.Request) (*embedding.Response, error) {
	if e == nil || e.protocol == nil {
		return nil, errors.New("vertexai: nil EmbeddingModel")
	}
	return e.protocol.Call(ctx, req)
}
