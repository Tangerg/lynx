package openai

import (
	"context"
	"errors"
	"iter"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"

	tts "github.com/Tangerg/lynx/core/speech"
	"github.com/Tangerg/lynx/models/internal/options"
	pkgio "github.com/Tangerg/lynx/pkg/io"
)

type AudioTTSModelConfig struct {
	APIKey         string
	DefaultOptions *tts.Options
	RequestOptions []option.RequestOption
}

func (c AudioTTSModelConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("openai: APIKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("openai: DefaultOptions is required")
	}
	return nil
}

var _ tts.Model = (*AudioTTSModel)(nil)
var _ tts.Streamer = (*AudioTTSModel)(nil)

type AudioTTSModel struct {
	api            *API
	defaultOptions *tts.Options
}

func NewAudioTTSModel(cfg AudioTTSModelConfig) (*AudioTTSModel, error) {
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

	return &AudioTTSModel{
		api:            api,
		defaultOptions: cfg.DefaultOptions,
	}, nil
}

func (a *AudioTTSModel) buildAPITTSRequest(req *tts.Request) (*openai.AudioSpeechNewParams, error) {
	mergedOpts, err := tts.MergeOptions(a.defaultOptions, req.Options)
	if err != nil {
		return nil, err
	}

	params := options.GetParams[openai.AudioSpeechNewParams](mergedOpts, OptionsKey)

	params.Model = mergedOpts.Model
	params.Input = req.Text
	// Each typed option only overrides Extra-threaded params when set —
	// empty strings / zero speed would clobber prior choices, and
	// Speed=0 is outside the API's 0.25–4.0 range.
	if mergedOpts.Voice != "" {
		params.Voice = openai.AudioSpeechNewParamsVoiceUnion{OfString: param.NewOpt(mergedOpts.Voice)}
	}
	if mergedOpts.Speed != 0 {
		params.Speed = openai.Float(mergedOpts.Speed)
	}
	if mergedOpts.ResponseFormat != "" {
		params.ResponseFormat = openai.AudioSpeechNewParamsResponseFormat(mergedOpts.ResponseFormat)
	}
	params.StreamFormat = openai.AudioSpeechNewParamsStreamFormatAudio

	return params, nil
}

func (a *AudioTTSModel) buildTTSResponse(data []byte) (*tts.Response, error) {
	result, err := tts.NewResult(data, &tts.ResultMetadata{})
	if err != nil {
		return nil, err
	}
	return tts.NewResponse(result, &tts.ResponseMetadata{})
}

func (a *AudioTTSModel) Call(ctx context.Context, req *tts.Request) (*tts.Response, error) {
	apiReq, err := a.buildAPITTSRequest(req)
	if err != nil {
		return nil, err
	}

	apiResp, err := a.api.AudioTTS(ctx, apiReq)
	if err != nil {
		return nil, err
	}
	defer apiResp.Body.Close()

	data, err := pkgio.ReadAll(apiResp.Body, 16*1024)
	if err != nil {
		return nil, err
	}

	return a.buildTTSResponse(data)
}

func (a *AudioTTSModel) Stream(ctx context.Context, req *tts.Request) iter.Seq2[*tts.Response, error] {
	return func(yield func(*tts.Response, error) bool) {
		apiReq, err := a.buildAPITTSRequest(req)
		if err != nil {
			yield(nil, err)
			return
		}

		apiResp, err := a.api.AudioTTS(ctx, apiReq)
		if err != nil {
			yield(nil, err)
			return
		}
		defer apiResp.Body.Close()

		for chunk, err := range pkgio.Read(apiResp.Body, 16*1024) {
			if err != nil {
				yield(nil, err)
				return
			}

			resp, err := a.buildTTSResponse(chunk)
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
