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

type AudioTranscriptionModelConfig struct {
	APIKey         model.APIKey
	DefaultOptions *transcription.Options
	RequestOptions []option.RequestOption

	// Metadata overrides the [transcription.ModelMetadata] returned by
	// [AudioTranscriptionModel.Metadata]. Zero Provider falls back to [Provider].
	Metadata *transcription.ModelMetadata
}

func (c AudioTranscriptionModelConfig) Validate() error {
	if c.APIKey == nil {
		return errors.New("openai: APIKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("openai: DefaultOptions is required")
	}
	return nil
}

var _ transcription.Model = (*AudioTranscriptionModel)(nil)

type AudioTranscriptionModel struct {
	api            *API
	defaultOptions *transcription.Options
	metadata       transcription.ModelMetadata
}

func NewAudioTranscriptionModel(cfg AudioTranscriptionModelConfig) (*AudioTranscriptionModel, error) {
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
	return &AudioTranscriptionModel{
		api:            api,
		defaultOptions: cfg.DefaultOptions,
		metadata:       info,
	}, nil
}

func (a *AudioTranscriptionModel) buildAPITranscriptionRequest(req *transcription.Request) (*openai.AudioTranscriptionNewParams, error) {
	mergedOpts, err := transcription.MergeOptions(a.defaultOptions, req.Options)
	if err != nil {
		return nil, err
	}

	params := options.GetParams[openai.AudioTranscriptionNewParams](mergedOpts, OptionsKey)

	params.Model = mergedOpts.Model
	if mergedOpts.Language != "" {
		params.Language = param.NewOpt(mergedOpts.Language)
	}
	if mergedOpts.Prompt != "" {
		params.Prompt = param.NewOpt(mergedOpts.Prompt)
	}
	if mergedOpts.Temperature != nil {
		params.Temperature = param.NewOpt(*mergedOpts.Temperature)
	}
	if mergedOpts.ResponseFormat != "" {
		params.ResponseFormat = openai.AudioResponseFormat(mergedOpts.ResponseFormat)
	}
	if len(mergedOpts.TimestampGranularity) > 0 {
		params.TimestampGranularities = mergedOpts.TimestampGranularity
	}

	data, err := req.Audio.Bytes()
	if err != nil {
		return nil, err
	}

	params.File = bytes.NewReader(data)

	return params, nil
}

func (a *AudioTranscriptionModel) buildTranscriptionResponse(resp *openai.AudioTranscriptionNewResponseUnion) (*transcription.Response, error) {
	result, err := transcription.NewResult(resp.Text, &transcription.ResultMetadata{})
	if err != nil {
		return nil, err
	}
	return transcription.NewResponse(result, &transcription.ResponseMetadata{})
}

func (a *AudioTranscriptionModel) Call(ctx context.Context, req *transcription.Request) (*transcription.Response, error) {
	apiReq, err := a.buildAPITranscriptionRequest(req)
	if err != nil {
		return nil, err
	}

	apiResp, err := a.api.AudioTranscription(ctx, apiReq)
	if err != nil {
		return nil, err
	}

	return a.buildTranscriptionResponse(apiResp)
}

func (a *AudioTranscriptionModel) DefaultOptions() transcription.Options {
	return *a.defaultOptions
}

func (a *AudioTranscriptionModel) Metadata() transcription.ModelMetadata {
	return a.metadata
}
