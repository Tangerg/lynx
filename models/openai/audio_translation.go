package openai

import (
	"bytes"
	"context"
	"errors"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/audio/transcription"
	"github.com/Tangerg/lynx/models/internal/options"
)

// AudioTranslationModelConfig configures the OpenAI /audio/translations
// backend. Only "whisper-1" is currently accepted by OpenAI — newer
// gpt-4o-transcribe models are transcription-only and reject translation
// calls.
type AudioTranslationModelConfig struct {
	APIKey         model.APIKey
	DefaultOptions *transcription.Options
	RequestOptions []option.RequestOption

	// Metadata overrides the [transcription.ModelMetadata] returned by
	// [AudioTranslationModel.Metadata]. Zero Provider falls back to [Provider].
	Metadata *transcription.ModelMetadata
}

func (c AudioTranslationModelConfig) Validate() error {
	if c.APIKey == nil {
		return errors.New("openai: APIKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("openai: DefaultOptions is required")
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
	defaultOptions *transcription.Options
	metadata       transcription.ModelMetadata
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

	info := transcription.ModelMetadata{Provider: Provider}
	if cfg.Metadata != nil {
		info = *cfg.Metadata
	}
	return &AudioTranslationModel{
		api:            api,
		defaultOptions: cfg.DefaultOptions,
		metadata:       info,
	}, nil
}

func (a *AudioTranslationModel) buildAPITranslationRequest(req *transcription.Request) (*openai.AudioTranslationNewParams, error) {
	mergedOpts, err := transcription.MergeOptions(a.defaultOptions, req.Options)
	if err != nil {
		return nil, err
	}

	params := options.GetParams[openai.AudioTranslationNewParams](mergedOpts, OptionsKey)

	params.Model = mergedOpts.Model
	if mergedOpts.Prompt != "" {
		params.Prompt = param.NewOpt(mergedOpts.Prompt)
	}
	if mergedOpts.Temperature != nil {
		params.Temperature = param.NewOpt(*mergedOpts.Temperature)
	}
	if mergedOpts.ResponseFormat != "" {
		params.ResponseFormat = openai.AudioTranslationNewParamsResponseFormat(mergedOpts.ResponseFormat)
	}

	data, err := req.Audio.DataAsBytes()
	if err != nil {
		return nil, err
	}
	params.File = bytes.NewReader(data)

	return params, nil
}

func (a *AudioTranslationModel) Call(ctx context.Context, req *transcription.Request) (*transcription.Response, error) {
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

func (a *AudioTranslationModel) DefaultOptions() transcription.Options {
	return *a.defaultOptions
}

func (a *AudioTranslationModel) Metadata() transcription.ModelMetadata {
	return a.metadata
}
