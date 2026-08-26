package vertexai

import (
	"context"
	"errors"
	"net/http"

	"google.golang.org/genai"

	"github.com/Tangerg/lynx/core/transcription"
	"github.com/Tangerg/lynx/models/google/internal/protocol"
)

type AudioTranscriptionModelConfig struct {
	Project        string
	Location       string
	DefaultOptions transcription.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (a AudioTranscriptionModelConfig) Validate() error {
	if a.Project == "" {
		return errors.New("vertexai: Project is required")
	}
	if a.Location == "" {
		return errors.New("vertexai: Location is required")
	}
	if a.DefaultOptions.Model == "" {
		return errors.New("vertexai: DefaultOptions.Model is required")
	}
	if _, err := a.DefaultOptions.Merged(); err != nil {
		return err
	}
	return nil
}

var _ transcription.Model = (*AudioTranscriptionModel)(nil)

type AudioTranscriptionModel struct {
	protocol *protocol.AudioTranscriptionModel
}

// NewAudioTranscriptionModel returns a Vertex AI transcription model.
func NewAudioTranscriptionModel(cfg AudioTranscriptionModelConfig) (*AudioTranscriptionModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	adapter, err := protocol.NewAudioTranscriptionModel(protocol.AudioTranscriptionModelConfig{
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
	return &AudioTranscriptionModel{protocol: adapter}, nil
}

func (a *AudioTranscriptionModel) Call(ctx context.Context, req *transcription.Request) (*transcription.Response, error) {
	if a == nil || a.protocol == nil {
		return nil, errors.New("vertexai: nil AudioTranscriptionModel")
	}
	return a.protocol.Call(ctx, req)
}
