package azureopenai

import (
	"context"
	"errors"

	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/core/transcription"
	"github.com/Tangerg/lynx/models/internal/protocol/openai"
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

var _ transcription.Model = (*AudioTranscriptionModel)(nil)

type AudioTranscriptionModel struct {
	protocol *openai.AudioTranscriptionModel
}

// NewAudioTranscriptionModel returns an Azure OpenAI transcription model.
func NewAudioTranscriptionModel(cfg AudioTranscriptionModelConfig) (*AudioTranscriptionModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	reqOpts, err := buildRequestOptions(cfg.BaseURL, cfg.RequestOptions)
	if err != nil {
		return nil, err
	}
	protocol, err := openai.NewAudioTranscriptionModel(openai.AudioTranscriptionModelConfig{
		Provider:       "azureopenai",
		APIKey:         cfg.APIKey,
		DefaultOptions: cfg.DefaultOptions,
		RequestOptions: reqOpts,
	})
	if err != nil {
		return nil, err
	}
	return &AudioTranscriptionModel{protocol: protocol}, nil
}

func (m *AudioTranscriptionModel) Call(ctx context.Context, req *transcription.Request) (*transcription.Response, error) {
	if m == nil || m.protocol == nil {
		return nil, errors.New("azureopenai: nil AudioTranscriptionModel")
	}
	return m.protocol.Call(ctx, req)
}
