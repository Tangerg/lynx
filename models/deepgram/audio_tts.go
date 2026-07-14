package deepgram

import (
	"bytes"
	"context"
	"errors"
	"iter"
	"net/http"

	"github.com/Tangerg/lynx/core/model"
	tts "github.com/Tangerg/lynx/core/speech"
	"github.com/Tangerg/lynx/models/internal/options"
	pkgio "github.com/Tangerg/lynx/pkg/io"
)

type AudioTTSModelConfig struct {
	APIKey         model.APIKey
	DefaultOptions *tts.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c AudioTTSModelConfig) Validate() error {
	if c.APIKey == nil {
		return errors.New("deepgram: APIKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("deepgram: DefaultOptions is required")
	}
	return nil
}

var _ tts.Model = (*AudioTTSModel)(nil)

// AudioTTSModel wraps Deepgram's /v1/speak endpoint. Supported models
// include the Aura family ("aura-asteria-en", "aura-luna-en", ...) and
// Aura-2 ("aura-2-thalia-en"); Deepgram uses model+voice fused as one
// id, so [tts.Options].Voice is unused and [tts.Options].Model carries
// the full picker.
type AudioTTSModel struct {
	api            *API
	defaultOptions *tts.Options
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
		defaultOptions: cfg.DefaultOptions,
	}, nil
}

func (a *AudioTTSModel) buildAPIRequest(req *tts.Request) (string, *SpeakParams, error) {
	mergedOpts, err := tts.MergeOptions(a.defaultOptions, req.Options)
	if err != nil {
		return "", nil, err
	}

	params := options.GetParams[SpeakParams](mergedOpts, OptionsKey)
	if params.Model == "" {
		params.Model = mergedOpts.Model
	}
	if params.Encoding == "" && mergedOpts.ResponseFormat != "" {
		params.Encoding = mergedOpts.ResponseFormat
	}

	return req.Text, params, nil
}

func (a *AudioTTSModel) buildResponse(audio []byte, hdr http.Header) (*tts.Response, error) {
	resultMeta := &tts.ResultMetadata{}
	if ct := hdr.Get("Content-Type"); ct != "" {
		resultMeta.Set("mime_type", ct)
	}
	if rid := hdr.Get("dg-request-id"); rid != "" {
		resultMeta.Set("request_id", rid)
	}

	result, err := tts.NewResult(audio, resultMeta)
	if err != nil {
		return nil, err
	}
	return tts.NewResponse(result, &tts.ResponseMetadata{})
}

func (a *AudioTTSModel) Call(ctx context.Context, req *tts.Request) (*tts.Response, error) {
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

// Stream re-uses /speak: the response body is chunked, so reading it in
// 16KB slices gives the same incremental playback shape callers get from
// the streaming-tts endpoint. (Deepgram has a separate websocket
// streaming endpoint; that's a different SPI and not surfaced here.)
func (a *AudioTTSModel) Stream(ctx context.Context, req *tts.Request) iter.Seq2[*tts.Response, error] {
	return func(yield func(*tts.Response, error) bool) {
		audio, hdr, err := func() ([]byte, http.Header, error) {
			text, params, err := a.buildAPIRequest(req)
			if err != nil {
				return nil, nil, err
			}
			return a.api.Speak(ctx, text, params)
		}()
		if err != nil {
			yield(nil, err)
			return
		}

		for chunk, err := range pkgio.Read(bytes.NewReader(audio), 16*1024) {
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
	return tts.ModelMetadata{Provider: Provider}
}
