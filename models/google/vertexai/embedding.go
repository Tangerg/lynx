package vertexai

import (
	"context"
	"errors"

	"google.golang.org/genai"

	"github.com/Tangerg/scope/core/embedding"
	"github.com/Tangerg/scope/models/google/internal/protocol"
)

type EmbeddingModelConfig struct {
	Client         ClientConfig
	DefaultOptions embedding.Options
}

func (e EmbeddingModelConfig) Validate() error {
	return e.Client.validateModelOptions(e.DefaultOptions.Model, e.DefaultOptions)
}

func (e EmbeddingModelConfig) protocol() protocol.EmbeddingModelConfig {
	return protocol.EmbeddingModelConfig{
		Provider:       protocolProvider,
		Backend:        genai.BackendVertexAI,
		Project:        e.Client.Project,
		Location:       e.Client.Location,
		DefaultOptions: e.DefaultOptions,
		BaseURL:        e.Client.BaseURL,
		HTTPClient:     e.Client.HTTPClient,
	}
}

var _ embedding.Model = (*EmbeddingModel)(nil)

type EmbeddingModel struct{ protocol *protocol.EmbeddingModel }

func NewEmbeddingModel(config EmbeddingModelConfig) (*EmbeddingModel, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	adapter, err := protocol.NewEmbeddingModel(config.protocol())
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
