package vertexai

import (
	"errors"
	"net/http"

	"google.golang.org/genai"

	tts "github.com/Tangerg/lynx/core/speech"
	"github.com/Tangerg/lynx/models/google"
)

type AudioTTSModelConfig struct {
	Project        string
	Location       string
	DefaultOptions tts.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c AudioTTSModelConfig) Validate() error {
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

// NewAudioTTSModel returns a [google.AudioTTSModel] backed by Vertex
// AI. Use a Gemini TTS model supported in the selected Vertex location.
func NewAudioTTSModel(cfg AudioTTSModelConfig) (*google.AudioTTSModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return google.NewAudioTTSModel(google.AudioTTSModelConfig{
		Backend:        genai.BackendVertexAI,
		Project:        cfg.Project,
		Location:       cfg.Location,
		DefaultOptions: cfg.DefaultOptions,
		BaseURL:        cfg.BaseURL,
		HTTPClient:     cfg.HTTPClient,
	})
}
