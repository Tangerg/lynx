package elevenlabs

import (
	"context"
	"errors"
	"iter"
	"net/http"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/audio/tts"
	"github.com/Tangerg/lynx/models/internal/options"
	pkgio "github.com/Tangerg/lynx/pkg/io"
)

type AudioTTSModelConfig struct {
	APIKey         model.APIKey
	DefaultOptions *tts.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c *AudioTTSModelConfig) validate() error {
	if c == nil {
		return errors.New("elevenlabs: config must not be nil")
	}
	if c.APIKey == nil {
		return errors.New("elevenlabs: APIKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("elevenlabs: DefaultOptions is required")
	}
	return nil
}

var _ tts.Model = (*AudioTTSModel)(nil)

// AudioTTSModel wraps ElevenLabs' /text-to-speech endpoint.
//
// ElevenLabs is voice-first: every call needs a voice id (the cloned /
// professional voice that says the text), so [tts.Options].Voice is
// required. [tts.Options].Model maps to ElevenLabs' model_id (e.g.
// "eleven_v3", "eleven_multilingual_v2") which selects the synthesis
// engine.
type AudioTTSModel struct {
	api            *API
	defaultOptions *tts.Options
}

func NewAudioTTSModel(cfg *AudioTTSModelConfig) (*AudioTTSModel, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	api, err := NewAPI(&APIConfig{
		APIKey:     cfg.APIKey,
		BaseURL:    cfg.BaseURL,
		HTTPClient: cfg.HTTPClient,
	})
	if err != nil {
		return nil, err
	}

	return &AudioTTSModel{
		api:            api,
		defaultOptions: cfg.DefaultOptions,
	}, nil
}

func (a *AudioTTSModel) buildAPIRequest(req *tts.Request) (voiceID, outputFormat string, body *TTSRequest, err error) {
	mergedOpts, mergeErr := tts.MergeOptions(a.defaultOptions, req.Options)
	if mergeErr != nil {
		return "", "", nil, mergeErr
	}

	if mergedOpts.Voice == "" {
		return "", "", nil, errors.New("elevenlabs: Voice (voice id) is required - set Options.Voice")
	}

	body = options.GetParams[TTSRequest](mergedOpts, OptionsKey)
	body.Text = req.Text
	body.ModelID = mergedOpts.Model

	if mergedOpts.Speed != 0 {
		// ElevenLabs voice speed is 0.7..1.2 in v3; let the API
		// reject out-of-range values rather than clamp here.
		if body.VoiceSettings == nil {
			body.VoiceSettings = &VoiceSettings{}
		}
		v := mergedOpts.Speed
		body.VoiceSettings.Speed = &v
	}

	return mergedOpts.Voice, mergedOpts.ResponseFormat, body, nil
}

func (a *AudioTTSModel) buildResponse(audio []byte, hdr http.Header) (*tts.Response, error) {
	resultMeta := &tts.ResultMetadata{}
	if ct := hdr.Get("Content-Type"); ct != "" {
		resultMeta.Set("mime_type", ct)
	}
	if rid := hdr.Get("request-id"); rid != "" {
		resultMeta.Set("request_id", rid)
	}

	result, err := tts.NewResult(audio, resultMeta)
	if err != nil {
		return nil, err
	}

	return tts.NewResponse(result, &tts.ResponseMetadata{})
}

func (a *AudioTTSModel) Call(ctx context.Context, req *tts.Request) (*tts.Response, error) {
	voiceID, outputFormat, body, err := a.buildAPIRequest(req)
	if err != nil {
		return nil, err
	}

	audio, hdr, err := a.api.TextToSpeech(ctx, voiceID, outputFormat, body)
	if err != nil {
		return nil, err
	}

	return a.buildResponse(audio, hdr)
}

func (a *AudioTTSModel) Stream(ctx context.Context, req *tts.Request) iter.Seq2[*tts.Response, error] {
	return func(yield func(*tts.Response, error) bool) {
		voiceID, outputFormat, body, err := a.buildAPIRequest(req)
		if err != nil {
			yield(nil, err)
			return
		}

		body_, hdr, err := a.api.TextToSpeechStream(ctx, voiceID, outputFormat, body)
		if err != nil {
			yield(nil, err)
			return
		}
		defer body_.Close()

		for chunk, err := range pkgio.Read(body_, 16*1024) {
			if err != nil {
				yield(nil, err)
				return
			}

			out, err := a.buildResponse(chunk, hdr)
			if err != nil {
				yield(nil, err)
				return
			}
			if !yield(out, nil) {
				return
			}
		}
	}
}

func (a *AudioTTSModel) DefaultOptions() tts.Options {
	return *a.defaultOptions
}

func (a *AudioTTSModel) Metadata() tts.ModelMetadata {
	return tts.ModelMetadata{
		Provider: Provider,
	}
}
