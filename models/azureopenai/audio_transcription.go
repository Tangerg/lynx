package azureopenai

import (
	"github.com/Tangerg/scope/core/transcription"
	"github.com/Tangerg/scope/models/protocol/openai"
)

type AudioTranscriptionModelConfig struct {
	Config
	DefaultOptions transcription.Options
}

func (a AudioTranscriptionModelConfig) resolve() (endpointConfig, error) {
	return resolveModelConfig(a.Config, a.DefaultOptions.Model, a.DefaultOptions.Validate)
}

func (a AudioTranscriptionModelConfig) Validate() error {
	_, err := a.resolve()
	return err
}

var _ transcription.Model = (*AudioTranscriptionModel)(nil)

type AudioTranscriptionModel = openai.AudioTranscriptionModel

func NewAudioTranscriptionModel(config AudioTranscriptionModelConfig) (*AudioTranscriptionModel, error) {
	endpoint, err := config.resolve()
	if err != nil {
		return nil, err
	}
	return openai.NewAudioTranscriptionModel(openai.AudioTranscriptionModelConfig{
		Provider:       protocolProvider,
		APIKey:         endpoint.apiKey,
		DefaultOptions: config.DefaultOptions,
		BaseURL:        endpoint.baseURL,
		HTTPClient:     endpoint.httpClient,
	})
}
