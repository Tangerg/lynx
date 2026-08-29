package azureopenai

import (
	"errors"

	tts "github.com/Tangerg/scope/core/speech"
	"github.com/Tangerg/scope/models/protocol/openai"
)

type AudioTTSModelConfig struct {
	Config
	DefaultOptions   tts.Options
	MaxResponseBytes int64
}

func (a AudioTTSModelConfig) resolve() (endpointConfig, error) {
	endpoint, err := resolveModelConfig(a.Config, a.DefaultOptions.Model, a.DefaultOptions.Validate)
	if err != nil {
		return endpointConfig{}, err
	}
	if a.MaxResponseBytes < 0 {
		return endpointConfig{}, errors.New("azureopenai: MaxResponseBytes must not be negative")
	}
	return endpoint, nil
}

func (a AudioTTSModelConfig) Validate() error {
	_, err := a.resolve()
	return err
}

var (
	_ tts.Model    = (*AudioTTSModel)(nil)
	_ tts.Streamer = (*AudioTTSModel)(nil)
)

type AudioTTSModel = openai.AudioTTSModel

func NewAudioTTSModel(config AudioTTSModelConfig) (*AudioTTSModel, error) {
	endpoint, err := config.resolve()
	if err != nil {
		return nil, err
	}
	return openai.NewAudioTTSModel(openai.AudioTTSModelConfig{
		Provider:         protocolProvider,
		APIKey:           endpoint.apiKey,
		DefaultOptions:   config.DefaultOptions,
		BaseURL:          endpoint.baseURL,
		HTTPClient:       endpoint.httpClient,
		MaxResponseBytes: config.MaxResponseBytes,
	})
}
