package azureopenai

import (
	"errors"
	"net/http"

	tts "github.com/Tangerg/scope/core/speech"
	"github.com/Tangerg/scope/models/protocol/openai"
)

type AudioTTSModelConfig struct {
	APIKey           string
	BaseURL          string
	DefaultOptions   tts.Options
	HTTPClient       *http.Client
	MaxResponseBytes int64
}

func (a AudioTTSModelConfig) Validate() error {
	if a.APIKey == "" {
		return errors.New("azureopenai: APIKey is required")
	}
	if a.DefaultOptions.Model == "" {
		return errors.New("azureopenai: DefaultOptions.Model is required")
	}
	if err := a.DefaultOptions.Validate(); err != nil {
		return err
	}
	if a.MaxResponseBytes < 0 {
		return errors.New("azureopenai: MaxResponseBytes must not be negative")
	}
	return nil
}

var (
	_ tts.Model    = (*AudioTTSModel)(nil)
	_ tts.Streamer = (*AudioTTSModel)(nil)
)

type AudioTTSModel = openai.AudioTTSModel

func NewAudioTTSModel(config AudioTTSModelConfig) (*AudioTTSModel, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	baseURL, err := normalizeBaseURL(config.BaseURL)
	if err != nil {
		return nil, err
	}
	return openai.NewAudioTTSModel(openai.AudioTTSModelConfig{
		Provider:         "azureopenai",
		APIKey:           config.APIKey,
		DefaultOptions:   config.DefaultOptions,
		BaseURL:          baseURL,
		HTTPClient:       config.HTTPClient,
		MaxResponseBytes: config.MaxResponseBytes,
	})
}
