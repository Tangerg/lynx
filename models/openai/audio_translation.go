package openai

import (
	"bytes"
	"context"
	"errors"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/core/transcription"
	"github.com/Tangerg/lynx/models/internal/options"
)

// AudioTranslationModelConfig configures the OpenAI /audio/translations
// backend. Only "whisper-1" is currently accepted by OpenAI — newer
// gpt-4o-transcribe models are transcription-only and reject translation
// calls.
type AudioTranslationModelConfig struct {
	APIKey         string
	DefaultOptions transcription.Options
	RequestOptions []option.RequestOption
}

func (c AudioTranslationModelConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("openai: APIKey is required")
	}
	if c.DefaultOptions.Model == "" {
		return errors.New("openai: DefaultOptions.Model is required")
	}
	if _, err := c.DefaultOptions.Merged(); err != nil {
		return err
	}
	return nil
}

var _ transcription.Model = (*AudioTranslationModel)(nil)

// AudioTranslationModel exposes OpenAI's /audio/translations endpoint —
// it accepts audio in any supported language and returns the **English**
// translation. The wire shape is the same as transcription (audio in,
// text out), so it implements the [transcription.Model] interface and
// can drop into any code path that already uses transcription.
//
// If the caller needs the original-language transcript instead of a
// translation, use [AudioTranscriptionModel].
type AudioTranslationModel struct {
	api            *API
	defaultOptions transcription.Options
}

func NewAudioTranslationModel(cfg AudioTranslationModelConfig) (*AudioTranslationModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	api, err := NewAPI(APIConfig{
		APIKey:         cfg.APIKey,
		RequestOptions: cfg.RequestOptions,
	})
	if err != nil {
		return nil, err
	}

	return &AudioTranslationModel{
		api:            api,
		defaultOptions: cfg.DefaultOptions.Clone(),
	}, nil
}

func (a *AudioTranslationModel) buildAPITranslationRequest(req *transcription.Request) (*openai.AudioTranslationNewParams, error) {
	mergedOpts, err := a.defaultOptions.Merged(req.Options)
	if err != nil {
		return nil, err
	}
	if err := options.RejectUnsupported("openai: translation", map[string]bool{
		"language": mergedOpts.Language != "",
	}); err != nil {
		return nil, err
	}

	params, err := options.GetParams[openai.AudioTranslationNewParams](mergedOpts.Extensions, OptionsKey)
	if err != nil {
		return nil, err
	}

	params.Model = mergedOpts.Model
	data, err := req.Audio.Bytes()
	if err != nil {
		return nil, err
	}
	params.File = bytes.NewReader(data)

	return params, nil
}

func (a *AudioTranslationModel) Call(ctx context.Context, req *transcription.Request) (*transcription.Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	apiReq, err := a.buildAPITranslationRequest(req)
	if err != nil {
		return nil, err
	}

	apiResp, err := a.api.AudioTranslation(ctx, apiReq)
	if err != nil {
		return nil, err
	}

	result, err := transcription.NewResult(apiResp.Text, &transcription.ResultMetadata{})
	if err != nil {
		return nil, err
	}
	return transcription.NewResponse(result, &transcription.ResponseMetadata{})
}
