package elevenlabs

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
		return errors.New("elevenlabs: APIKey is required")
	}
	if c.DefaultOptions.Model == "" {
		return errors.New("elevenlabs: DefaultOptions.Model is required")
	}
	if _, err := c.DefaultOptions.Merged(); err != nil {
		return err
	}
	return nil
}

var _ tts.Model = (*AudioTTSModel)(nil)
var _ tts.Streamer = (*AudioTTSModel)(nil)

// AudioTTSModel wraps ElevenLabs' /text-to-speech endpoint.
//
// ElevenLabs is voice-first: every call needs a voice id (the cloned /
// professional voice that says the text), so [tts.Options].Voice is
// required. [tts.Options].Model maps to ElevenLabs' model_id (e.g.
// "eleven_v3", "eleven_multilingual_v2") which selects the synthesis
// engine.
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

func (a *AudioTTSModel) buildAPIRequest(req *tts.Request) (voiceID, outputFormat string, body *ttsRequest, err error) {
	mergedOpts, mergeErr := a.defaultOptions.Merged(req.Options)
	if mergeErr != nil {
		return "", "", nil, mergeErr
	}

	if mergedOpts.Voice == "" {
		return "", "", nil, errors.New("elevenlabs: Voice (voice id) is required - set Options.Voice")
	}

	bodyValue, _, err := metadata.Decode[ttsRequest](mergedOpts.Extensions, SpeechRequestExtensionKey)
	if err != nil {
		return "", "", nil, err
	}
	body = &bodyValue
	body.Text = req.Text
	body.ModelID = mergedOpts.Model

	if mergedOpts.Speed != 0 {
		if mergedOpts.Speed < 0.7 || mergedOpts.Speed > 1.2 {
			return "", "", nil, fmt.Errorf("elevenlabs: speech speed must be between 0.7 and 1.2, got %g", mergedOpts.Speed)
		}
		if body.VoiceSettings == nil {
			body.VoiceSettings = &voiceSettings{}
		}
		v := mergedOpts.Speed
		body.VoiceSettings.Speed = &v
	}
	if body.OptimizeStreamingLatency != nil && (*body.OptimizeStreamingLatency < 0 || *body.OptimizeStreamingLatency > 4) {
		return "", "", nil, fmt.Errorf("elevenlabs: optimize_streaming_latency must be between 0 and 4, got %d", *body.OptimizeStreamingLatency)
	}
	if mergedOpts.OutputFormat != "" && !isSupportedOutputFormat(mergedOpts.OutputFormat) {
		return "", "", nil, fmt.Errorf("elevenlabs: unsupported output format %q", mergedOpts.OutputFormat)
	}

	return mergedOpts.Voice, mergedOpts.OutputFormat, body, nil
}

func (a *AudioTTSModel) buildResponse(audio []byte, hdr http.Header) (*tts.Response, error) {
	if len(audio) == 0 {
		return nil, errors.New("elevenlabs: speech response contained no audio")
	}
	resultMeta := &tts.ResultMetadata{}
	if ct := hdr.Get("Content-Type"); ct != "" {
		if err := resultMeta.Set("elevenlabs/mime_type", ct); err != nil {
			return nil, err
		}
	}

	result, err := tts.NewResult(audio, resultMeta)
	if err != nil {
		return nil, err
	}

	responseMetadata := &tts.ResponseMetadata{}
	for key, value := range map[string]string{
		"elevenlabs/character_cost": hdr.Get("character-cost"),
		"elevenlabs/request_id":     hdr.Get("request-id"),
		"elevenlabs/trace_id":       hdr.Get("x-trace-id"),
	} {
		if value != "" {
			if err := responseMetadata.Set(key, value); err != nil {
				return nil, err
			}
		}
	}
	if err := responseMetadata.Set(SpeechResponseExtensionKey, map[string]any{
		"content_type":   hdr.Get("Content-Type"),
		"character_cost": hdr.Get("character-cost"),
		"request_id":     hdr.Get("request-id"),
		"trace_id":       hdr.Get("x-trace-id"),
	}); err != nil {
		return nil, err
	}
	return tts.NewResponse(result, responseMetadata)
}

func isSupportedOutputFormat(format string) bool {
	switch format {
	case "alaw_8000",
		"mp3_22050_32", "mp3_24000_48", "mp3_44100_32", "mp3_44100_64", "mp3_44100_96", "mp3_44100_128", "mp3_44100_192",
		"opus_48000_32", "opus_48000_64", "opus_48000_96", "opus_48000_128", "opus_48000_192",
		"pcm_8000", "pcm_16000", "pcm_22050", "pcm_24000", "pcm_32000", "pcm_44100", "pcm_48000",
		"ulaw_8000",
		"wav_8000", "wav_16000", "wav_22050", "wav_24000", "wav_32000", "wav_44100", "wav_48000":
		return true
	default:
		return false
	}
}

func (a *AudioTTSModel) Call(ctx context.Context, req *tts.Request) (*tts.Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	voiceID, outputFormat, body, err := a.buildAPIRequest(req)
	if err != nil {
		return nil, err
	}

	audio, hdr, err := a.api.textToSpeech(ctx, voiceID, outputFormat, body)
	if err != nil {
		return nil, err
	}

	return a.buildResponse(audio, hdr)
}

func (a *AudioTTSModel) Stream(ctx context.Context, req *tts.Request) iter.Seq2[*tts.Response, error] {
	return func(yield func(*tts.Response, error) bool) {
		if err := req.Validate(); err != nil {
			yield(nil, err)
			return
		}
		voiceID, outputFormat, body, err := a.buildAPIRequest(req)
		if err != nil {
			yield(nil, err)
			return
		}

		body_, hdr, err := a.api.textToSpeechStream(ctx, voiceID, outputFormat, body)
		if err != nil {
			yield(nil, err)
			return
		}
		defer body_.Close()

		for chunk, err := range streamio.Read(body_) {
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
