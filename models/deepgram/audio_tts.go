package deepgram

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"net/http"

	tts "github.com/Tangerg/lynx/core/speech"
)

type AudioTTSModelConfig struct {
	APIKey         string
	DefaultOptions tts.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (a AudioTTSModelConfig) Validate() error {
	if a.APIKey == "" {
		return errors.New("deepgram: APIKey is required")
	}
	if a.DefaultOptions.Model == "" {
		return errors.New("deepgram: DefaultOptions.Model is required")
	}
	if err := a.DefaultOptions.Validate(); err != nil {
		return err
	}
	return nil
}

var _ tts.Model = (*AudioTTSModel)(nil)
var _ tts.Streamer = (*AudioTTSModel)(nil)

// AudioTTSModel wraps Deepgram's /v1/speak endpoint. Supported models
// include the Aura family ("aura-asteria-en", "aura-luna-en", ...) and
// Aura-2 ("aura-2-thalia-en"); Deepgram uses model+voice fused as one
// id, so [tts.Options].Voice is unused and [tts.Options].Model carries
// the full picker.
type AudioTTSModel struct {
	api            *api
	defaultOptions tts.Options
}

func NewAudioTTSModel(cfg AudioTTSModelConfig) (*AudioTTSModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	api, err := newAPI(apiConfig{
		APIKey:     cfg.APIKey,
		BaseURL:    cfg.BaseURL,
		HTTPClient: cfg.HTTPClient,
	})
	if err != nil {
		return nil, err
	}

	return &AudioTTSModel{
		api:            api,
		defaultOptions: cfg.DefaultOptions.Clone(),
	}, nil
}

func (a *AudioTTSModel) buildAPIRequest(req *tts.Request) (string, *speakParams, error) {
	effectiveOptions, err := a.defaultOptions.Resolve(req.Options)
	if err != nil {
		return "", nil, err
	}
	if effectiveOptions.Voice != "" {
		return "", nil, errors.New("deepgram: speech: unsupported option: voice")
	}

	paramsValue, _, err := effectiveOptions.Extensions.Decode[speakParams](SpeechRequestExtensionKey)

	params := &paramsValue
	if err != nil {
		return "", nil, err
	}
	if params.Model == "" {
		params.Model = effectiveOptions.Model
	}
	if effectiveOptions.OutputFormat != "" {
		switch effectiveOptions.OutputFormat {
		case "wav":
			params.Encoding = "linear16"
			params.Container = "wav"
		case "mp3", "flac", "aac", "opus", "mulaw", "alaw", "linear16":
			params.Encoding = effectiveOptions.OutputFormat
		default:
			return "", nil, fmt.Errorf("deepgram: speech: unsupported output format %q", effectiveOptions.OutputFormat)
		}
	}
	if effectiveOptions.Speed != 0 {
		if effectiveOptions.Speed < 0.7 || effectiveOptions.Speed > 1.5 {
			return "", nil, errors.New("deepgram: speech: speed must be between 0.7 and 1.5")
		}
		params.Speed = effectiveOptions.Speed
	}

	return req.Text, params, nil
}

func (a *AudioTTSModel) buildResponse(audio []byte, hdr http.Header) (*tts.Response, error) {
	if len(audio) == 0 {
		return nil, errors.New("deepgram: speech response contained no audio")
	}
	outputMetadata := &tts.OutputMetadata{}
	if ct := hdr.Get("Content-Type"); ct != "" {
		if err := outputMetadata.Set("deepgram/mime_type", ct); err != nil {
			return nil, err
		}
	}

	output, err := tts.NewOutput(audio, outputMetadata)
	if err != nil {
		return nil, err
	}
	meta := &tts.ResponseMetadata{Model: hdr.Get("dg-model-name")}
	if requestID := hdr.Get("dg-request-id"); requestID != "" {
		if err := meta.Set("deepgram/request_id", requestID); err != nil {
			return nil, err
		}
	}
	if err := meta.Set(SpeechResponseExtensionKey, map[string]string{
		"content_type": hdr.Get("Content-Type"),
		"model_name":   hdr.Get("dg-model-name"),
		"request_id":   hdr.Get("dg-request-id"),
	}); err != nil {
		return nil, err
	}
	return tts.NewResponse(output, meta)
}

func (a *AudioTTSModel) Call(ctx context.Context, req *tts.Request) (*tts.Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	text, params, err := a.buildAPIRequest(req)
	if err != nil {
		return nil, err
	}

	audio, hdr, err := a.api.speak(ctx, text, params)
	if err != nil {
		return nil, err
	}

	return a.buildResponse(audio, hdr)
}

// Stream exposes the chunked response body from Deepgram's official REST
// /speak endpoint as it arrives.
func (a *AudioTTSModel) Stream(ctx context.Context, req *tts.Request) iter.Seq2[*tts.Response, error] {
	return func(yield func(*tts.Response, error) bool) {
		if err := req.Validate(); err != nil {
			yield(nil, err)
			return
		}
		text, params, err := a.buildAPIRequest(req)
		if err != nil {
			yield(nil, err)
			return
		}
		body, hdr, err := a.api.speakStream(ctx, text, params)
		if err != nil {
			yield(nil, err)
			return
		}
		defer body.Close()

		for chunk, err := range readAudioChunks(body) {
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
