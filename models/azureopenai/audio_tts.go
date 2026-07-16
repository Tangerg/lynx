package azureopenai

import (
	"errors"

	"github.com/openai/openai-go/v3/option"

	tts "github.com/Tangerg/lynx/core/speech"
	"github.com/Tangerg/lynx/models/openai"
)

type AudioTTSModelConfig struct {
	APIKey         string
	Endpoint       string
	APIVersion     string
	DefaultOptions tts.Options
	RequestOptions []option.RequestOption
}

func (c AudioTTSModelConfig) Validate() error {
	if c.Endpoint == "" {
		return errors.New("azureopenai: Endpoint is required")
	}
	if c.DefaultOptions.Model == "" {
		return errors.New("azureopenai: DefaultOptions.Model is required")
	}
	if _, err := c.DefaultOptions.Merged(); err != nil {
		return err
	}
	return nil
}

// NewAudioTTSModel returns an [openai.AudioTTSModel] pointed at Azure
// OpenAI's /audio/speech endpoint. [tts.Options].Model is the Azure
// deployment id (typically pointing at "tts-1" / "tts-1-hd" /
// "gpt-4o-mini-tts").
func NewAudioTTSModel(cfg AudioTTSModelConfig) (*openai.AudioTTSModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	apiKey, reqOpts := buildAzureRequestOptions(cfg.APIKey, cfg.Endpoint, cfg.APIVersion, cfg.RequestOptions)
	return openai.NewAudioTTSModel(openai.AudioTTSModelConfig{
		APIKey:         apiKey,
		DefaultOptions: cfg.DefaultOptions,
		RequestOptions: reqOpts,
	})
}
