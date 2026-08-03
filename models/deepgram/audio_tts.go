package deepgram

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"net/http"

	"github.com/Tangerg/lynx/core/metadata"
	tts "github.com/Tangerg/lynx/core/speech"
	"github.com/Tangerg/lynx/models/internal/options"
	"github.com/Tangerg/lynx/models/internal/streamio"
)

type AudioTTSModelConfig struct {
	APIKey         string
	DefaultOptions tts.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c AudioTTSModelConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("deepgram: APIKey is required")
	}
	if c.DefaultOptions.Model == "" {
		return errors.New("deepgram: DefaultOptions.Model is required")
	}
	if _, err := c.DefaultOptions.Merged(); err != nil {
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
	api            *API
	defaultOptions tts.Options
}

func NewAudioTTSModel(cfg AudioTTSModelConfig) (*AudioTTSModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	api, err := NewAPI(APIConfig{
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

func (a *AudioTTSModel) buildAPIRequest(req *tts.Request) (string, *SpeakParams, error) {
	mergedOpts, err := a.defaultOptions.Merged(req.Options)
	if err != nil {
		return "", nil, err
	}
	if err := options.RejectUnsupported("deepgram: speech", map[string]bool{
		"voice": mergedOpts.Voice != "",
	}); err != nil {
		return "", nil, err
	}

	paramsValue, _, err := metadata.Decode[SpeakParams](mergedOpts.Extensions, SpeechRequestExtensionKey)

	params := &paramsValue
	if err != nil {
		return "", nil, err
	}
	if params.Model == "" {
		params.Model = mergedOpts.Model
	}
	if mergedOpts.OutputFormat != "" {
		switch mergedOpts.OutputFormat {
		case "wav":
			params.Encoding = "linear16"
			params.Container = "wav"
		case "mp3", "flac", "aac", "opus", "mulaw", "alaw", "linear16":
			params.Encoding = mergedOpts.OutputFormat
		default:
			return "", nil, fmt.Errorf("deepgram: speech: unsupported output format %q", mergedOpts.OutputFormat)
		}
	}
	if mergedOpts.Speed != 0 {
		if mergedOpts.Speed < 0.7 || mergedOpts.Speed > 1.5 {
			return "", nil, errors.New("deepgram: speech: speed must be between 0.7 and 1.5")
		}
		params.Speed = mergedOpts.Speed
	}

	return req.Text, params, nil
}

func (a *AudioTTSModel) buildResponse(audio []byte, hdr http.Header) (*tts.Response, error) {
	if len(audio) == 0 {
		return nil, errors.New("deepgram: speech response contained no audio")
	}
	resultMeta := &tts.ResultMetadata{}
	if ct := hdr.Get("Content-Type"); ct != "" {
		if err := resultMeta.Set("deepgram/mime_type", ct); err != nil {
			return nil, err
		}
	}

	result, err := tts.NewResult(audio, resultMeta)
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
	return tts.NewResponse(result, meta)
}

func (a *AudioTTSModel) Call(ctx context.Context, req *tts.Request) (*tts.Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	text, params, err := a.buildAPIRequest(req)
	if err != nil {
		return nil, err
	}

	audio, hdr, err := a.api.Speak(ctx, text, params)
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
		body, hdr, err := a.api.SpeakStream(ctx, text, params)
		if err != nil {
			yield(nil, err)
			return
		}
		defer body.Close()

		for chunk, err := range streamio.Read(body) {
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
