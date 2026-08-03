package lmnt

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"net/http"

	"github.com/Tangerg/lynx/core/metadata"
	tts "github.com/Tangerg/lynx/core/speech"
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
// The official bytes endpoint streams a binary HTTP response. Call buffers it;
// Stream exposes provider chunks as they arrive.
type AudioTTSModel struct {
	api            *api
	defaultOptions tts.Options
}

func NewAudioTTSModel(cfg AudioTTSModelConfig) (*AudioTTSModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	api, err := newAPI(apiConfig{APIKey: cfg.APIKey, BaseURL: cfg.BaseURL, HTTPClient: cfg.HTTPClient})
	if err != nil {
		return nil, err
	}
	return &AudioTTSModel{api: api, defaultOptions: cfg.DefaultOptions.Clone()}, nil
}

func (a *AudioTTSModel) buildAPIRequest(req *tts.Request) (*synthesizeRequest, error) {
	mergedOpts, err := a.defaultOptions.Merged(req.Options)
	if err != nil {
		return nil, err
	}

	bodyValue, _, err := metadata.Decode[synthesizeRequest](mergedOpts.Extensions, RequestExtensionKey)

	body := &bodyValue
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
	if mergedOpts.Speed != 0 {
		return nil, errors.New("lmnt: options.speed is not supported by the current speech bytes API")
	}
	if err := validateSynthesizeRequest(body); err != nil {
		return nil, err
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
	audio, headers, err := a.api.synthesize(ctx, body)
	if err != nil {
		return nil, err
	}
	return buildResponse(audio, headers, body.Model)
}

func buildResponse(audio []byte, headers http.Header, model string) (*tts.Response, error) {
	if len(audio) == 0 {
		return nil, errors.New("lmnt: speech response contained no audio")
	}
	resultMetadata := &tts.ResultMetadata{}
	if contentType := headers.Get("Content-Type"); contentType != "" {
		if err := resultMetadata.Set("lmnt/mime_type", contentType); err != nil {
			return nil, err
		}
	}
	result, err := tts.NewResult(audio, resultMetadata)
	if err != nil {
		return nil, err
	}
	metadata := &tts.ResponseMetadata{Model: model}
	if requestID := headers.Get("request-id"); requestID != "" {
		if err := metadata.Set("lmnt/request_id", requestID); err != nil {
			return nil, err
		}
	}
	if err := metadata.Set(ResponseExtensionKey, map[string]string{
		"content_type": headers.Get("Content-Type"),
		"request_id":   headers.Get("request-id"),
	}); err != nil {
		return nil, err
	}
	return tts.NewResponse(result, metadata)
}

func (a *AudioTTSModel) Stream(ctx context.Context, req *tts.Request) iter.Seq2[*tts.Response, error] {
	return func(yield func(*tts.Response, error) bool) {
		if err := req.Validate(); err != nil {
			yield(nil, err)
			return
		}
		request, err := a.buildAPIRequest(req)
		if err != nil {
			yield(nil, err)
			return
		}
		body, headers, err := a.api.synthesizeStream(ctx, request)
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
			response, err := buildResponse(chunk, headers, request.Model)
			if err != nil {
				yield(nil, err)
				return
			}
			if !yield(response, nil) {
				return
			}
		}
	}
}

func validateSynthesizeRequest(req *synthesizeRequest) error {
	if req.Model != ModelBlizzard {
		return fmt.Errorf("lmnt: speech model must be %q, got %q", ModelBlizzard, req.Model)
	}
	if req.Voice == "" {
		return errors.New("lmnt: speech voice is required")
	}
	if len(req.Text) > 5000 {
		return fmt.Errorf("lmnt: speech text exceeds the 5000-character limit: %d", len(req.Text))
	}
	if req.Format != "" {
		switch req.Format {
		case "aac", "mp3", "ulaw", "wav", "webm", "pcm_s16le", "pcm_f32le":
		default:
			return fmt.Errorf("lmnt: unsupported speech format %q", req.Format)
		}
	}
	if req.SampleRate != 0 && req.SampleRate != 8000 && req.SampleRate != 16000 && req.SampleRate != 24000 {
		return fmt.Errorf("lmnt: sample_rate must be 8000, 16000, or 24000, got %d", req.SampleRate)
	}
	if req.Temperature != nil && *req.Temperature < 0 {
		return fmt.Errorf("lmnt: temperature must be non-negative, got %g", *req.Temperature)
	}
	if req.TopP != nil && (*req.TopP < 0 || *req.TopP > 1) {
		return fmt.Errorf("lmnt: top_p must be between 0 and 1, got %g", *req.TopP)
	}
	return nil
}
