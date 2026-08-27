package lmnt

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"net/http"

	tts "github.com/Tangerg/scope/core/speech"
)

type AudioTTSModelConfig struct {
	APIKey         string
	DefaultOptions tts.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (a AudioTTSModelConfig) Validate() error {
	if a.APIKey == "" {
		return errors.New("lmnt: APIKey is required")
	}
	if a.DefaultOptions.Model == "" {
		return errors.New("lmnt: DefaultOptions.Model is required")
	}
	if err := a.DefaultOptions.Validate(); err != nil {
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
	effectiveOptions, err := a.defaultOptions.Resolve(req.Options)
	if err != nil {
		return nil, err
	}

	bodyValue, _, err := effectiveOptions.Extensions.Decode[synthesizeRequest](RequestExtensionKey)

	body := &bodyValue
	if err != nil {
		return nil, err
	}
	body.Text = req.Text
	if body.Model == "" {
		body.Model = effectiveOptions.Model
	}
	if body.Voice == "" {
		body.Voice = effectiveOptions.Voice
	}
	if body.Format == "" && effectiveOptions.OutputFormat != "" {
		body.Format = effectiveOptions.OutputFormat
	}
	if effectiveOptions.Speed != 0 {
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
	outputMetadata := &tts.OutputMetadata{}
	if contentType := headers.Get("Content-Type"); contentType != "" {
		if err := outputMetadata.Set("lmnt/mime_type", contentType); err != nil {
			return nil, err
		}
	}
	output, err := tts.NewOutput(audio, outputMetadata)
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
	return tts.NewResponse(output, metadata)
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
		for chunk, err := range readAudioChunks(body) {
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
