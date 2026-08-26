package azureopenai

import (
	"errors"
	"net/http"

	tts "github.com/Tangerg/lynx/core/speech"
	"github.com/Tangerg/lynx/models/protocol/openai"
)

type AudioTTSModelConfig struct {
	APIKey         string
	BaseURL        string
	DefaultOptions tts.Options
	HTTPClient     *http.Client
}

func (a AudioTTSModelConfig) Validate() error {
	if a.APIKey == "" {
		return errors.New("azureopenai: APIKey is required")
	}
	if a.DefaultOptions.Model == "" {
		return errors.New("azureopenai: DefaultOptions.Model is required")
	}
	if _, err := a.DefaultOptions.Merged(); err != nil {
		return err
	}
	return nil
}

var (
	_ tts.Model    = (*AudioTTSModel)(nil)
	_ tts.Streamer = (*AudioTTSModel)(nil)
)

type AudioTTSModel = openai.AudioTTSModel

// NewAudioTTSModel returns an Azure OpenAI speech model.
func NewAudioTTSModel(cfg AudioTTSModelConfig) (*AudioTTSModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	baseURL, err := normalizeBaseURL(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	return openai.NewAudioTTSModel(openai.AudioTTSModelConfig{
		Provider:       "azureopenai",
		APIKey:         cfg.APIKey,
		DefaultOptions: cfg.DefaultOptions,
		BaseURL:        baseURL,
		HTTPClient:     cfg.HTTPClient,
	})
}
