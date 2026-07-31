package azureopenai

import (
	"errors"

	"github.com/openai/openai-go/v3/option"

	tts "github.com/Tangerg/lynx/core/speech"
	"github.com/Tangerg/lynx/models/openai"
)

type AudioTTSModelConfig struct {
	APIKey         string
	BaseURL        string
	DefaultOptions tts.Options
	RequestOptions []option.RequestOption
}

func (c AudioTTSModelConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("azureopenai: APIKey is required")
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
// OpenAI's v1 /audio/speech endpoint. [tts.Options].Model is the Azure
// deployment id (typically pointing at "tts-1" / "tts-1-hd" /
// "gpt-4o-mini-tts").
func NewAudioTTSModel(cfg AudioTTSModelConfig) (*openai.AudioTTSModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	reqOpts, err := buildRequestOptions(cfg.BaseURL, cfg.RequestOptions)
	if err != nil {
		return nil, err
	}
	return openai.NewAudioTTSModel(openai.AudioTTSModelConfig{
		APIKey:         cfg.APIKey,
		DefaultOptions: cfg.DefaultOptions,
		RequestOptions: reqOpts,
	})
}
