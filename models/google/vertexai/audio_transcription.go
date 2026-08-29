package vertexai

import (
	"github.com/Tangerg/scope/core/transcription"
	"github.com/Tangerg/scope/models/google/internal/protocol"
)

type AudioTranscriptionModelConfig struct {
	Client         ClientConfig
	DefaultOptions transcription.Options
}

func (a AudioTranscriptionModelConfig) Validate() error {
	return a.protocol().Validate()
}

func (a AudioTranscriptionModelConfig) protocol() protocol.AudioTranscriptionModelConfig {
	return protocol.AudioTranscriptionModelConfig{
		Provider:       protocolProvider,
		Client:         a.Client.protocol(),
		DefaultOptions: a.DefaultOptions,
	}
}

var _ transcription.Model = (*AudioTranscriptionModel)(nil)

type AudioTranscriptionModel = callModel[transcription.Request, transcription.Response]

func NewAudioTranscriptionModel(config AudioTranscriptionModelConfig) (*AudioTranscriptionModel, error) {
	return newCallModel[transcription.Request, transcription.Response](protocol.NewAudioTranscriptionModel(config.protocol()))
}
