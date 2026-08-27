package vertexai

import (
	"context"
	"errors"

	"google.golang.org/genai"

	"github.com/Tangerg/scope/core/transcription"
	"github.com/Tangerg/scope/models/google/internal/protocol"
)

type AudioTranscriptionModelConfig struct {
	Client         ClientConfig
	DefaultOptions transcription.Options
}

func (a AudioTranscriptionModelConfig) Validate() error {
	return a.Client.validateModelOptions(a.DefaultOptions.Model, a.DefaultOptions)
}

func (a AudioTranscriptionModelConfig) protocol() protocol.AudioTranscriptionModelConfig {
	return protocol.AudioTranscriptionModelConfig{
		Provider:       protocolProvider,
		Backend:        genai.BackendVertexAI,
		Project:        a.Client.Project,
		Location:       a.Client.Location,
		DefaultOptions: a.DefaultOptions,
		BaseURL:        a.Client.BaseURL,
		HTTPClient:     a.Client.HTTPClient,
	}
}

var _ transcription.Model = (*AudioTranscriptionModel)(nil)

type AudioTranscriptionModel struct {
	protocol *protocol.AudioTranscriptionModel
}

func NewAudioTranscriptionModel(config AudioTranscriptionModelConfig) (*AudioTranscriptionModel, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	adapter, err := protocol.NewAudioTranscriptionModel(config.protocol())
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
