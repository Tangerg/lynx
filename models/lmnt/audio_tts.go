package lmnt

import (
	"context"
	"errors"
	"iter"
	"net/http"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/audio/tts"
	"github.com/Tangerg/lynx/models/internal/options"
)

type AudioTTSModelConfig struct {
	ApiKey         model.ApiKey
	DefaultOptions *tts.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c *AudioTTSModelConfig) validate() error {
	if c == nil {
		return errors.New("lmnt: config must not be nil")
	}
	if c.ApiKey == nil {
		return errors.New("lmnt: ApiKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("lmnt: DefaultOptions is required")
	}
	return nil
}

var _ tts.Model = (*AudioTTSModel)(nil)

// AudioTTSModel wraps LMNT's /v1/ai/speech/bytes endpoint. LMNT pairs
// (model_id, voice_id) for each call; [tts.Options].Model picks the
// engine ("aurora", "blizzard", ...) and [tts.Options].Voice picks the
// voice id.
//
// LMNT also exposes a websocket streaming endpoint; that's a different
// SPI shape and not surfaced here. Stream() returns the full audio as a
// single chunk for shape compatibility.
type AudioTTSModel struct {
	api            *Api
	defaultOptions *tts.Options
}

func NewAudioTTSModel(cfg *AudioTTSModelConfig) (*AudioTTSModel, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	api, err := NewApi(&ApiConfig{ApiKey: cfg.ApiKey, BaseURL: cfg.BaseURL, HTTPClient: cfg.HTTPClient})
	if err != nil {
		return nil, err
	}
	return &AudioTTSModel{api: api, defaultOptions: cfg.DefaultOptions}, nil
}

func (a *AudioTTSModel) buildApiRequest(req *tts.Request) (*SynthesizeRequest, error) {
	mergedOpts, err := tts.MergeOptions(a.defaultOptions, req.Options)
	if err != nil {
		return nil, err
	}

	body := options.GetParams[SynthesizeRequest](mergedOpts, OptionsKey)
	body.Text = req.Text
	if body.Model == "" {
		body.Model = mergedOpts.Model
	}
	if body.Voice == "" {
		body.Voice = mergedOpts.Voice
	}
	if body.Format == "" && mergedOpts.ResponseFormat != "" {
		body.Format = mergedOpts.ResponseFormat
	}
	if body.Speed == nil && mergedOpts.Speed != 0 {
		v := mergedOpts.Speed
		body.Speed = &v
	}
	return body, nil
}

func (a *AudioTTSModel) Call(ctx context.Context, req *tts.Request) (*tts.Response, error) {
	body, err := a.buildApiRequest(req)
	if err != nil {
		return nil, err
	}
	apiResp, err := a.api.Synthesize(ctx, body)
	if err != nil {
		return nil, err
	}
	audio, err := apiResp.Decode()
	if err != nil {
		return nil, err
	}
	resultMeta := &tts.ResultMetadata{}
	if apiResp.Seed != 0 {
		resultMeta.Set("seed", apiResp.Seed)
	}
	if len(apiResp.Durations) > 0 {
		resultMeta.Set("durations", apiResp.Durations)
	}
	result, err := tts.NewResult(audio, resultMeta)
	if err != nil {
		return nil, err
	}
	return tts.NewResponse(result, &tts.ResponseMetadata{})
}

func (a *AudioTTSModel) Stream(ctx context.Context, req *tts.Request) iter.Seq2[*tts.Response, error] {
	return func(yield func(*tts.Response, error) bool) {
		resp, err := a.Call(ctx, req)
		if err != nil {
			yield(nil, err)
			return
		}
		yield(resp, nil)
	}
}

func (a *AudioTTSModel) DefaultOptions() tts.Options { return *a.defaultOptions }
func (a *AudioTTSModel) Metadata() tts.ModelMetadata         { return tts.ModelMetadata{Provider: Provider} }
