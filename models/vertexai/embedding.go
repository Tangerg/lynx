package vertexai

import (
	"errors"
	"net/http"

	"google.golang.org/genai"

	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/models/google"
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

// NewEmbeddingModel returns a [google.EmbeddingModel] backed by Vertex AI.
// Select a model that implements Vertex's EmbedContent contract.
func NewEmbeddingModel(cfg EmbeddingModelConfig) (*google.EmbeddingModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return google.NewEmbeddingModel(google.EmbeddingModelConfig{
		Backend:        genai.BackendVertexAI,
		Project:        cfg.Project,
		Location:       cfg.Location,
		DefaultOptions: cfg.DefaultOptions,
		BaseURL:        cfg.BaseURL,
		HTTPClient:     cfg.HTTPClient,
	})
}
