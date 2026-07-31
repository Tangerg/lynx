package azureopenai

import (
	"errors"

	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/core/transcription"
	"github.com/Tangerg/lynx/models/openai"
)

type AudioTranscriptionModelConfig struct {
	APIKey         string
	BaseURL        string
	DefaultOptions transcription.Options
	RequestOptions []option.RequestOption
}

func (c AudioTranscriptionModelConfig) Validate() error {
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

// NewAudioTranscriptionModel returns an [openai.AudioTranscriptionModel]
// pointed at Azure OpenAI's v1 /audio/transcriptions endpoint.
// [transcription.Options].Model is the Azure deployment id (typically
// pointing at "whisper" / "gpt-4o-transcribe" / "gpt-4o-mini-transcribe").
func NewAudioTranscriptionModel(cfg AudioTranscriptionModelConfig) (*openai.AudioTranscriptionModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	reqOpts, err := buildRequestOptions(cfg.BaseURL, cfg.RequestOptions)
	if err != nil {
		return nil, err
	}
	return openai.NewAudioTranscriptionModel(openai.AudioTranscriptionModelConfig{
		APIKey:         cfg.APIKey,
		DefaultOptions: cfg.DefaultOptions,
		RequestOptions: reqOpts,
	})
}
