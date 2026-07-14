package azureopenai

import (
	"errors"

	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/transcription"
	"github.com/Tangerg/lynx/models/openai"
)

type AudioTranscriptionModelConfig struct {
	APIKey         model.APIKey
	Endpoint       string
	APIVersion     string
	DefaultOptions *transcription.Options
	RequestOptions []option.RequestOption
}

func (c AudioTranscriptionModelConfig) Validate() error {
	if c.Endpoint == "" {
		return errors.New("azureopenai: Endpoint is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("azureopenai: DefaultOptions is required")
	}
	return nil
}

// NewAudioTranscriptionModel returns an [openai.AudioTranscriptionModel]
// pointed at Azure OpenAI's /audio/transcriptions endpoint.
// [transcription.Options].Model is the Azure deployment id (typically
// pointing at "whisper" / "gpt-4o-transcribe" / "gpt-4o-mini-transcribe").
func NewAudioTranscriptionModel(cfg AudioTranscriptionModelConfig) (*openai.AudioTranscriptionModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	apiKey, reqOpts := buildAzureRequestOptions(cfg.APIKey, cfg.Endpoint, cfg.APIVersion, cfg.RequestOptions)
	return openai.NewAudioTranscriptionModel(openai.AudioTranscriptionModelConfig{
		APIKey:         apiKey,
		DefaultOptions: cfg.DefaultOptions,
		RequestOptions: reqOpts,
	})
}
