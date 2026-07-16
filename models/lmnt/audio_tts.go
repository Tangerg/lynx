package lmnt

import (
	"context"
	"errors"
	"iter"
	"net/http"

	tts "github.com/Tangerg/lynx/core/speech"
	"github.com/Tangerg/lynx/models/internal/options"
)

type AudioTTSModelConfig struct {
	APIKey         string
	DefaultOptions tts.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c AudioTTSModelConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("lmnt: APIKey is required")
	}
	if c.DefaultOptions.Model == "" {
		return errors.New("lmnt: DefaultOptions.Model is required")
	}
	if _, err := c.DefaultOptions.Merged(); err != nil {
		return err
	}
	return nil
}

var _ tts.Model = (*AudioTTSModel)(nil)
var _ tts.Streamer = (*AudioTTSModel)(nil)

// AudioTTSModel wraps LMNT's /v1/ai/speech/bytes endpoint. LMNT pairs
// (model_id, voice_id) for each call; [tts.Options].Model picks the
// engine ("aurora", "blizzard", ...) and [tts.Options].Voice picks the
// voice id.
//
// LMNT also exposes a websocket streaming endpoint; that's a different
// SPI shape and not surfaced here. Stream() returns the full audio as a
// single chunk for shape compatibility.
type AudioTTSModel struct {
	api            *API
	defaultOptions tts.Options
}

func NewAudioTTSModel(cfg AudioTTSModelConfig) (*AudioTTSModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	api, err := NewAPI(APIConfig{APIKey: cfg.APIKey, BaseURL: cfg.BaseURL, HTTPClient: cfg.HTTPClient})
	if err != nil {
		return nil, err
	}
	return &AudioTTSModel{api: api, defaultOptions: cfg.DefaultOptions.Clone()}, nil
}

func (a *AudioTTSModel) buildAPIRequest(req *tts.Request) (*SynthesizeRequest, error) {
	mergedOpts, err := a.defaultOptions.Merged(req.Options)
	if err != nil {
		return nil, err
	}

	body, err := options.GetParams[SynthesizeRequest](mergedOpts.Extensions, OptionsKey)
	if err != nil {
		return nil, err
	}
	body.Text = req.Text
	if body.Model == "" {
		body.Model = mergedOpts.Model
	}
	if body.Voice == "" {
		body.Voice = mergedOpts.Voice
	}
	if body.Format == "" && mergedOpts.OutputFormat != "" {
		body.Format = mergedOpts.OutputFormat
	}
	if body.Speed == nil && mergedOpts.Speed != 0 {
		v := mergedOpts.Speed
		body.Speed = &v
	}
	return body, nil
}

func (a *AudioTTSModel) Call(ctx context.Context, req *tts.Request) (*tts.Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	body, err := a.buildAPIRequest(req)
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
		if err := resultMeta.Set("seed", apiResp.Seed); err != nil {
			return nil, err
		}
	}
	if len(apiResp.Durations) > 0 {
		if err := resultMeta.Set("durations", apiResp.Durations); err != nil {
			return nil, err
		}
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
