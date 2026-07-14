package vertexai

import (
	"errors"

	"google.golang.org/genai"

	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/models/google"
)

type EmbeddingModelConfig struct {
	Project        string
	Location       string
	DefaultOptions *embedding.Options
}

func (c EmbeddingModelConfig) Validate() error {
	if c.Project == "" {
		return errors.New("vertexai: Project is required")
	}
	if c.Location == "" {
		return errors.New("vertexai: Location is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("vertexai: DefaultOptions is required")
	}
	return nil
}

// NewEmbeddingModel returns a [google.EmbeddingModel] backed by
// Vertex AI. Supported models: text-embedding-005,
// text-multilingual-embedding-002, gemini-embedding-001.
func NewEmbeddingModel(cfg EmbeddingModelConfig) (*google.EmbeddingModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return google.NewEmbeddingModel(google.EmbeddingModelConfig{
		Backend:        genai.BackendVertexAI,
		Project:        cfg.Project,
		Location:       cfg.Location,
		DefaultOptions: cfg.DefaultOptions,
	})
}
