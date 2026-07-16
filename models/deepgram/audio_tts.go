package deepgram

import (
	"bytes"
	"context"
	"errors"
	"iter"
	"net/http"

	tts "github.com/Tangerg/lynx/core/speech"
	"github.com/Tangerg/lynx/models/internal/options"
	pkgio "github.com/Tangerg/lynx/pkg/io"
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
		"speed": mergedOpts.Speed != 0,
		"voice": mergedOpts.Voice != "",
	}); err != nil {
		return "", nil, err
	}

	params, err := options.GetParams[SpeakParams](mergedOpts.Extensions, OptionsKey)
	if err != nil {
		return "", nil, err
	}
	if params.Model == "" {
		params.Model = mergedOpts.Model
	}
	if params.Encoding == "" && mergedOpts.OutputFormat != "" {
		params.Encoding = mergedOpts.OutputFormat
	}

	return req.Text, params, nil
}

func (a *AudioTTSModel) buildResponse(audio []byte, hdr http.Header) (*tts.Response, error) {
	resultMeta := &tts.ResultMetadata{}
	if ct := hdr.Get("Content-Type"); ct != "" {
		if err := resultMeta.Set("mime_type", ct); err != nil {
			return nil, err
		}
	}
	if rid := hdr.Get("dg-request-id"); rid != "" {
		if err := resultMeta.Set("request_id", rid); err != nil {
			return nil, err
		}
	}

	result, err := tts.NewResult(audio, resultMeta)
	if err != nil {
		return nil, err
	}
	return tts.NewResponse(result, &tts.ResponseMetadata{})
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

// Stream re-uses /speak: the response body is chunked, so reading it in
// 16KB slices gives the same incremental playback shape callers get from
// the streaming-tts endpoint. (Deepgram has a separate websocket
// streaming endpoint; that's a different SPI and not surfaced here.)
func (a *AudioTTSModel) Stream(ctx context.Context, req *tts.Request) iter.Seq2[*tts.Response, error] {
	return func(yield func(*tts.Response, error) bool) {
		if err := req.Validate(); err != nil {
			yield(nil, err)
			return
		}
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
