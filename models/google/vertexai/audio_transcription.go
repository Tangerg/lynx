package vertexai

import (
	"context"
	"errors"
	"net/http"

	"google.golang.org/genai"

	"github.com/Tangerg/lynx/core/transcription"
	google "github.com/Tangerg/lynx/models/google/internal/protocol"
)

type AudioTranscriptionModelConfig struct {
	Project        string
	Location       string
	DefaultOptions transcription.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c AudioTranscriptionModelConfig) Validate() error {
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

var _ transcription.Model = (*AudioTranscriptionModel)(nil)

type AudioTranscriptionModel struct {
	protocol *google.AudioTranscriptionModel
}

// NewAudioTranscriptionModel returns a Vertex AI transcription model.
func NewAudioTranscriptionModel(cfg AudioTranscriptionModelConfig) (*AudioTranscriptionModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	protocol, err := google.NewAudioTranscriptionModel(google.AudioTranscriptionModelConfig{
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
	return &AudioTranscriptionModel{protocol: protocol}, nil
}

func (m *AudioTranscriptionModel) Call(ctx context.Context, req *transcription.Request) (*transcription.Response, error) {
	if m == nil || m.protocol == nil {
		return nil, errors.New("vertexai: nil AudioTranscriptionModel")
	}
	return m.protocol.Call(ctx, req)
}
