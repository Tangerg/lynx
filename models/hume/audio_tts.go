package hume

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
	APIKey         model.APIKey
	DefaultOptions *tts.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c AudioTTSModelConfig) Validate() error {
	if c.APIKey == nil {
		return errors.New("hume: APIKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("hume: DefaultOptions is required")
	}
	return nil
}

var _ tts.Model = (*AudioTTSModel)(nil)

// AudioTTSModel wraps Hume's Octave TTS (/v0/tts). Hume's headline
// feature is emotion-aware synthesis driven by per-utterance
// "description" cues — those live on the Extra-threaded [TTSRequest].
//
// [tts.Options].Voice maps onto a HUME_AI voice id (Octave preset);
// [tts.Options].Model is unused (Octave is the only engine here).
type AudioTTSModel struct {
	api            *API
	defaultOptions *tts.Options
}

func NewAudioTTSModel(cfg AudioTTSModelConfig) (*AudioTTSModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	api, err := NewAPI(APIConfig{APIKey: cfg.APIKey, BaseURL: cfg.BaseURL, HTTPClient: cfg.HTTPClient})
	if err != nil {
		return nil, err
	}
	return &AudioTTSModel{api: api, defaultOptions: cfg.DefaultOptions}, nil
}

func (a *AudioTTSModel) buildAPIRequest(req *tts.Request) (*TTSRequest, error) {
	mergedOpts, err := tts.MergeOptions(a.defaultOptions, req.Options)
	if err != nil {
		return nil, err
	}

	body := options.GetParams[TTSRequest](mergedOpts, OptionsKey)
	if len(body.Utterances) == 0 {
		utt := Utterance{Text: req.Text}
		if mergedOpts.Voice != "" {
			utt.Voice = &Voice{ID: mergedOpts.Voice, Provider: "HUME_AI"}
		}
		if mergedOpts.Speed != 0 {
			v := mergedOpts.Speed
			utt.Speed = &v
		}
		body.Utterances = []Utterance{utt}
	}
	if body.Format == nil && mergedOpts.ResponseFormat != "" {
		body.Format = map[string]any{"type": mergedOpts.ResponseFormat}
	}
	return body, nil
}

func (a *AudioTTSModel) Call(ctx context.Context, req *tts.Request) (*tts.Response, error) {
	body, err := a.buildAPIRequest(req)
	if err != nil {
		return nil, err
	}
	apiResp, err := a.api.TTS(ctx, body)
	if err != nil {
		return nil, err
	}
	audio, err := apiResp.DecodeAudio()
	if err != nil {
		return nil, err
	}
	resultMeta := &tts.ResultMetadata{}
	if len(apiResp.Generations) > 0 {
		g := apiResp.Generations[0]
		resultMeta.Set("mime_type", "audio/"+g.Encoding.Format)
		resultMeta.Set("sample_rate", g.Encoding.SampleRate)
		resultMeta.Set("duration", g.Duration)
		resultMeta.Set("generation_id", g.ID)
	}
	result, err := tts.NewResult(audio, resultMeta)
	if err != nil {
		return nil, err
	}
	meta := &tts.ResponseMetadata{}
	if apiResp.RequestID != "" {
		meta.Set("request_id", apiResp.RequestID)
	}
	return tts.NewResponse(result, meta)
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
