package vertexai

import (
	"errors"

	"google.golang.org/genai"

	"github.com/Tangerg/lynx/core/image"
	"github.com/Tangerg/lynx/models/google"
)

type ImageModelConfig struct {
	Project        string
	Location       string
	DefaultOptions image.Options
}

func (c ImageModelConfig) Validate() error {
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

// NewImageModel returns a [google.ImageModel] backed by Vertex AI.
// Supported models: imagen-4.0-generate-001, imagen-3.0-generate-002.
func NewImageModel(cfg ImageModelConfig) (*google.ImageModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return google.NewImageModel(google.ImageModelConfig{
		Backend:        genai.BackendVertexAI,
		Project:        cfg.Project,
		Location:       cfg.Location,
		DefaultOptions: cfg.DefaultOptions,
	})
}
