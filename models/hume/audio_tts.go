package hume

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
		return errors.New("hume: APIKey is required")
	}
	if c.DefaultOptions.Model == "" {
		return errors.New("hume: DefaultOptions.Model is required")
	}
	if _, err := c.DefaultOptions.Merged(); err != nil {
		return err
	}
	return nil
}

var _ tts.Model = (*AudioTTSModel)(nil)
var _ tts.Streamer = (*AudioTTSModel)(nil)

// AudioTTSModel wraps Hume's Octave TTS (/v0/tts). Hume's headline
// feature is emotion-aware synthesis driven by per-utterance
// "description" cues — those live on the extension-threaded [TTSRequest].
//
// [tts.Options].Voice maps onto a HUME_AI voice id (Octave preset);
// [tts.Options].Model is unused (Octave is the only engine here).
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

func (a *AudioTTSModel) buildAPIRequest(req *tts.Request) (*TTSRequest, error) {
	mergedOpts, err := a.defaultOptions.Merged(req.Options)
	if err != nil {
		return nil, err
	}

	body, err := options.GetParams[TTSRequest](mergedOpts.Extensions, OptionsKey)
	if err != nil {
		return nil, err
	}
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
	if body.Format == nil && mergedOpts.OutputFormat != "" {
		body.Format = map[string]any{"type": mergedOpts.OutputFormat}
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
		if err := resultMeta.Set("mime_type", "audio/"+g.Encoding.Format); err != nil {
			return nil, err
		}
		if err := resultMeta.Set("sample_rate", g.Encoding.SampleRate); err != nil {
			return nil, err
		}
		if err := resultMeta.Set("duration", g.Duration); err != nil {
			return nil, err
		}
		if err := resultMeta.Set("generation_id", g.ID); err != nil {
			return nil, err
		}
	}
	result, err := tts.NewResult(audio, resultMeta)
	if err != nil {
		return nil, err
	}
	meta := &tts.ResponseMetadata{}
	if apiResp.RequestID != "" {
		if err := meta.Set("request_id", apiResp.RequestID); err != nil {
			return nil, err
		}
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
