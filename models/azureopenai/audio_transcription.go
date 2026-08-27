package azureopenai

import (
	"errors"
	"net/http"

	"github.com/Tangerg/scope/core/transcription"
	"github.com/Tangerg/scope/models/protocol/openai"
)

type AudioTranscriptionModelConfig struct {
	APIKey         string
	BaseURL        string
	DefaultOptions transcription.Options
	HTTPClient     *http.Client
}

func (a AudioTranscriptionModelConfig) Validate() error {
	if a.APIKey == "" {
		return errors.New("azureopenai: APIKey is required")
	}
	if a.DefaultOptions.Model == "" {
		return errors.New("azureopenai: DefaultOptions.Model is required")
	}
	if err := a.DefaultOptions.Validate(); err != nil {
		return err
	}
	return nil
}

var _ transcription.Model = (*AudioTranscriptionModel)(nil)

type AudioTranscriptionModel = openai.AudioTranscriptionModel

func NewAudioTranscriptionModel(config AudioTranscriptionModelConfig) (*AudioTranscriptionModel, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	baseURL, err := normalizeBaseURL(config.BaseURL)
	if err != nil {
		return nil, err
	}
	return openai.NewAudioTranscriptionModel(openai.AudioTranscriptionModelConfig{
		Provider:       "azureopenai",
		APIKey:         config.APIKey,
		DefaultOptions: config.DefaultOptions,
		BaseURL:        baseURL,
		HTTPClient:     config.HTTPClient,
	})
}
