package openai

import (
	"context"
	"errors"
	"iter"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/ai/model"
	"github.com/Tangerg/lynx/ai/model/audio/tts"
	pkgio "github.com/Tangerg/lynx/pkg/io"
)

type AudioTTSModelConfig struct {
	ApiKey         model.ApiKey
	DefaultOptions *tts.Options
	RequestOptions []option.RequestOption
}

func (c *AudioTTSModelConfig) validate() error {
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

var _ tts.Model = (*AudioTTSModel)(nil)

type AudioTTSModel struct {
	api            *Api
	defaultOptions *tts.Options
}

func NewAudioTTSModel(cfg *AudioTTSModelConfig) (*AudioTTSModel, error) {
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
	return &AudioTTSModel{
		api:            api,
		defaultOptions: cfg.DefaultOptions,
	}, nil
}

func (a *AudioTTSModel) buildApiTTSRequest(req *tts.Request) (*openai.AudioSpeechNewParams, error) {
	mergedOpts, err := tts.MergeOptions(a.defaultOptions, req.Options)
	if err != nil {
		return nil, err
	}

	params := getOptionsParams[openai.AudioSpeechNewParams](mergedOpts)

	params.Model = mergedOpts.Model

	params.Input = req.Text

	params.Voice = openai.AudioSpeechNewParamsVoice(mergedOpts.Voice)

	params.Speed = openai.Float(mergedOpts.Speed)

	params.ResponseFormat = openai.AudioSpeechNewParamsResponseFormat(mergedOpts.ResponseFormat)

	params.StreamFormat = openai.AudioSpeechNewParamsStreamFormatAudio

	return params, nil
}

func (a *AudioTTSModel) buildTTSResponse(data []byte) (*tts.Response, error) {
	result, err := tts.NewResult(data, &tts.ResultMetadata{})
	if err != nil {
		return nil, err
	}
	return tts.NewResponse([]*tts.Result{result}, &tts.ResponseMetadata{})
}

func (a *AudioTTSModel) Call(ctx context.Context, req *tts.Request) (*tts.Response, error) {
	apiReq, err := a.buildApiTTSRequest(req)
	if err != nil {
		return nil, err
	}

	apiResp, err := a.api.AudioTextToSpeech(ctx, apiReq)
	if err != nil {
		return nil, err
	}
	defer apiResp.Body.Close()

	bytes, err := pkgio.ReadAll(apiResp.Body, 16*1024)
	if err != nil {
		return nil, err
	}

	return a.buildTTSResponse(bytes)
}

func (a *AudioTTSModel) Stream(ctx context.Context, req *tts.Request) iter.Seq2[*tts.Response, error] {
	return func(yield func(*tts.Response, error) bool) {
		apiReq, err := a.buildApiTTSRequest(req)
		if err != nil {
			yield(nil, err)
			return
		}

		apiResp, err := a.api.AudioTextToSpeech(ctx, apiReq)
		if err != nil {
			yield(nil, err)
			return
		}

		defer apiResp.Body.Close()

		for bytes, err := range pkgio.Read(apiResp.Body, 16*1024) {
			if err != nil {
				yield(nil, err)
				return
			}
			resp, err := a.buildTTSResponse(bytes)
			if err != nil {
				yield(nil, err)
				return
			}
			if !yield(resp, nil) {
				return
			}
		}
	}
}

func (a *AudioTTSModel) DefaultOptions() *tts.Options {
	return a.defaultOptions
}

func (a *AudioTTSModel) Info() tts.ModelInfo {
	return tts.ModelInfo{
		Provider: Provider,
	}
}
