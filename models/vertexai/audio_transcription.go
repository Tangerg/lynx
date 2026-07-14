package vertexai

import (
	"errors"

	"google.golang.org/genai"

	"github.com/Tangerg/lynx/core/transcription"
	"github.com/Tangerg/lynx/models/google"
)

type AudioTranscriptionModelConfig struct {
	Project        string
	Location       string
	DefaultOptions *transcription.Options
}

func (c AudioTranscriptionModelConfig) Validate() error {
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

// NewAudioTranscriptionModel returns a [google.AudioTranscriptionModel]
// backed by Vertex AI — Gemini's multimodal chat used through the
// transcription interface.
func NewAudioTranscriptionModel(cfg AudioTranscriptionModelConfig) (*google.AudioTranscriptionModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return google.NewAudioTranscriptionModel(google.AudioTranscriptionModelConfig{
		Backend:        genai.BackendVertexAI,
		Project:        cfg.Project,
		Location:       cfg.Location,
		DefaultOptions: cfg.DefaultOptions,
		Metadata:       &transcription.ModelMetadata{Provider: Provider},
	})
}
