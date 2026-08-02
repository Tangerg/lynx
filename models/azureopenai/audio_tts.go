package azureopenai

import (
	"context"
	"errors"
	"iter"

	"github.com/openai/openai-go/v3/option"

	tts "github.com/Tangerg/lynx/core/speech"
	"github.com/Tangerg/lynx/models/internal/protocol/openai"
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

var (
	_ tts.Model    = (*AudioTTSModel)(nil)
	_ tts.Streamer = (*AudioTTSModel)(nil)
)

type AudioTTSModel struct{ protocol *openai.AudioTTSModel }

// NewAudioTTSModel returns an Azure OpenAI speech model.
func NewAudioTTSModel(cfg AudioTTSModelConfig) (*AudioTTSModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	reqOpts, err := buildRequestOptions(cfg.BaseURL, cfg.RequestOptions)
	if err != nil {
		return nil, err
	}
	protocol, err := openai.NewAudioTTSModel(openai.AudioTTSModelConfig{
		Provider:       "azureopenai",
		APIKey:         cfg.APIKey,
		DefaultOptions: cfg.DefaultOptions,
		RequestOptions: reqOpts,
	})
	if err != nil {
		return nil, err
	}
	return &AudioTTSModel{protocol: protocol}, nil
}

func (m *AudioTTSModel) Call(ctx context.Context, req *tts.Request) (*tts.Response, error) {
	if m == nil || m.protocol == nil {
		return nil, errors.New("azureopenai: nil AudioTTSModel")
	}
	return m.protocol.Call(ctx, req)
}

func (m *AudioTTSModel) Stream(ctx context.Context, req *tts.Request) iter.Seq2[*tts.Response, error] {
	if m == nil || m.protocol == nil {
		return func(yield func(*tts.Response, error) bool) { yield(nil, errors.New("azureopenai: nil AudioTTSModel")) }
	}
	if err := req.Validate(); err != nil {
		return func(yield func(*tts.Response, error) bool) { yield(nil, err) }
	}
	return m.protocol.Stream(ctx, req)
}
