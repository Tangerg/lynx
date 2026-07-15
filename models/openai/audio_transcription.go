package openai

import (
	"bytes"
	"context"
	"errors"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"

	"github.com/Tangerg/lynx/core/transcription"
	"github.com/Tangerg/lynx/models/internal/options"
)

type AudioTranscriptionModelConfig struct {
	APIKey         string
	DefaultOptions *transcription.Options
	RequestOptions []option.RequestOption
}

func (c AudioTranscriptionModelConfig) Validate() error {
	if c.APIKey == "" {
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

	return &AudioTranscriptionModel{
		api:            api,
		defaultOptions: cfg.DefaultOptions,
	}, nil
}

func (a *AudioTranscriptionModel) buildAPITranscriptionRequest(req *transcription.Request) (*openai.AudioTranscriptionNewParams, error) {
	mergedOpts, err := a.defaultOptions.Merged(req.Options)
	if err != nil {
		return nil, err
	}

	params, err := options.GetParams[openai.AudioTranscriptionNewParams](mergedOpts.Extra, OptionsKey)
	if err != nil {
		return nil, err
	}

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
	if err := req.Validate(); err != nil {
		return nil, err
	}
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
