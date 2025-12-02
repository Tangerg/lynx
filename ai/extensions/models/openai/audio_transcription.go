package openai

import (
	"bytes"
	"context"
	"errors"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/ai/model"
	"github.com/Tangerg/lynx/ai/model/audio/transcription"
)

type AudioTranscriptionModelConfig struct {
	ApiKey         model.ApiKey
	DefaultOptions *transcription.Options
	RequestOptions []option.RequestOption
}

func (c *AudioTranscriptionModelConfig) validate() error {
	if c == nil {
		return errors.New("config is nil")
	}
	if c.ApiKey == nil {
		return errors.New("apiKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("default options cannot be nil")
	}
	return nil
}

var _ transcription.Model = (*AudioTranscriptionModel)(nil)

type AudioTranscriptionModel struct {
	api            *Api
	defaultOptions *transcription.Options
}

func NewAudioTranscriptionModel(cfg *AudioTranscriptionModelConfig) (*AudioTranscriptionModel, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	api, err := NewApi(&ApiConfig{
		ApiKey:         cfg.ApiKey,
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

func (a *AudioTranscriptionModel) buildApiTranscriptionRequest(req *transcription.Request) (*openai.AudioTranscriptionNewParams, error) {
	mergedOpts, err := transcription.MergeOptions(a.defaultOptions, req.Options)
	if err != nil {
		return nil, err
	}

	params := getOptionsParams[openai.AudioTranscriptionNewParams](mergedOpts)

	params.Model = mergedOpts.Model

	data, err := req.Audio.DataAsBytes()
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
	return transcription.NewResponse([]*transcription.Result{result}, &transcription.ResponseMetadata{})
}

func (a *AudioTranscriptionModel) Call(ctx context.Context, req *transcription.Request) (*transcription.Response, error) {
	apiReq, err := a.buildApiTranscriptionRequest(req)
	if err != nil {
		return nil, err
	}

	apiResp, err := a.api.AudioTranscription(ctx, apiReq)
	if err != nil {
		return nil, err
	}

	return a.buildTranscriptionResponse(apiResp)
}

func (a *AudioTranscriptionModel) DefaultOptions() *transcription.Options {
	return a.defaultOptions
}

func (a *AudioTranscriptionModel) Info() transcription.ModelInfo {
	return transcription.ModelInfo{
		Provider: Provider,
	}
}
