package hume

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
// "description" cues — those live on the extension-threaded provider request.
//
// [tts.Options].Voice maps onto a HUME_AI voice id and
// [tts.Options].Model selects the official Octave version ("1" or "2").
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

func (a *AudioTTSModel) buildAPIRequest(req *tts.Request, streaming bool) (*ttsRequest, error) {
	mergedOpts, err := a.defaultOptions.Merged(req.Options)
	if err != nil {
		return nil, err
	}

	bodyValue, _, err := mergedOpts.Extensions.Decode[ttsRequest](SpeechRequestExtensionKey)

	body := &bodyValue
	if err != nil {
		return nil, err
	}
	if mergedOpts.Model != ModelOctave1 && mergedOpts.Model != ModelOctave2 {
		return nil, fmt.Errorf("hume: speech: model must be %q or %q", ModelOctave1, ModelOctave2)
	}
	if body.NumGenerations > 1 {
		return nil, errors.New("hume: speech: num_generations greater than 1 cannot be represented by Core's single-result response")
	}
	if len(body.Utterances) == 0 {
		body.Utterances = []utterance{{}}
	}
	body.Utterances[0].Text = req.Text
	if mergedOpts.Voice != "" {
		body.Utterances[0].Voice = &voice{ID: mergedOpts.Voice, Provider: "HUME_AI"}
	}
	if mergedOpts.Speed != 0 {
		v := mergedOpts.Speed
		body.Utterances[0].Speed = &v
	}
	body.Version = mergedOpts.Model
	if mergedOpts.OutputFormat != "" {
		switch mergedOpts.OutputFormat {
		case "mp3", "wav", "pcm":
		default:
			return nil, errors.New("hume: speech: output_format must be mp3, wav, or pcm")
		}
		body.Format = map[string]any{"type": mergedOpts.OutputFormat}
	}
	if body.Version == ModelOctave2 && body.Utterances[0].Voice == nil {
		return nil, errors.New("hume: speech: Octave 2 requires Options.Voice or a voice on the first utterance")
	}
	if !streaming && body.InstantMode != nil {
		return nil, errors.New("hume: speech: instant_mode is only supported by streaming endpoints")
	}
	if streaming && body.Utterances[0].Voice == nil && body.InstantMode == nil {
		instantMode := false
		body.InstantMode = &instantMode
	}
	return body, nil
}

func (a *AudioTTSModel) Call(ctx context.Context, req *tts.Request) (*tts.Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	body, err := a.buildAPIRequest(req, false)
	if err != nil {
		return nil, err
	}
	apiResp, err := a.api.tts(ctx, body)
	if err != nil {
		return nil, err
	}
	return a.buildResponse(apiResp, body.Version)
}

func (a *AudioTTSModel) buildResponse(apiResp *ttsResponse, model string) (*tts.Response, error) {
	audio, err := apiResp.DecodeAudio()
	if err != nil {
		return nil, err
	}
	resultMeta := &tts.ResultMetadata{}
	if len(apiResp.Generations) > 0 {
		g := apiResp.Generations[0]
		if err := resultMeta.Set("hume/encoding_format", g.Encoding.Format); err != nil {
			return nil, err
		}
		if err := resultMeta.Set("hume/sample_rate", g.Encoding.SampleRate); err != nil {
			return nil, err
		}
		if err := resultMeta.Set("hume/duration_seconds", g.Duration); err != nil {
			return nil, err
		}
		if err := resultMeta.Set("hume/generation_id", g.ID); err != nil {
			return nil, err
		}
	}
	result, err := tts.NewResult(audio, resultMeta)
	if err != nil {
		return nil, err
	}
	meta := &tts.ResponseMetadata{Model: model}
	if apiResp.RequestID != "" {
		if err := meta.Set("hume/request_id", apiResp.RequestID); err != nil {
			return nil, err
		}
	}
	if err := meta.Set("hume/response", apiResp); err != nil {
		return nil, err
	}
	return tts.NewResponse(result, meta)
}

func (a *AudioTTSModel) Stream(ctx context.Context, req *tts.Request) iter.Seq2[*tts.Response, error] {
	return func(yield func(*tts.Response, error) bool) {
		if err := req.Validate(); err != nil {
			yield(nil, err)
			return
		}
		request, err := a.buildAPIRequest(req, true)
		if err != nil {
			yield(nil, err)
			return
		}
		if len(request.IncludeTimestampTypes) != 0 {
			yield(nil, errors.New("hume: speech stream: include_timestamp_types cannot be represented by Core audio-only stream responses"))
			return
		}
		body, err := a.api.ttsStream(ctx, request)
		if err != nil {
			yield(nil, err)
			return
		}
		defer body.Close()

		decoder := json.NewDecoder(body)
		for {
			var event ttsStreamEvent
			if err := decoder.Decode(&event); err != nil {
				if errors.Is(err, io.EOF) {
					return
				}
				if contextErr := ctx.Err(); contextErr != nil {
					yield(nil, contextErr)
					return
				}
				yield(nil, fmt.Errorf("hume: decode streamed JSON response: %w", err))
				return
			}
			if event.Type != "audio" {
				yield(nil, fmt.Errorf("hume: unexpected streamed event type %q", event.Type))
				return
			}
			response, err := buildStreamResponse(&event, request.Version)
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

func buildStreamResponse(event *ttsStreamEvent, model string) (*tts.Response, error) {
	audio, err := event.DecodeAudio()
	if err != nil {
		return nil, err
	}
	resultMetadata := &tts.ResultMetadata{}
	for key, value := range map[string]any{
		"hume/audio_format":       event.AudioFormat,
		"hume/chunk_index":        event.ChunkIndex,
		"hume/generation_id":      event.GenerationID,
		"hume/is_last_chunk":      event.IsLastChunk,
		"hume/snippet_id":         event.SnippetID,
		"hume/text":               event.Text,
		"hume/transcribed_text":   event.TranscribedText,
		"hume/utterance_index":    event.UtteranceIndex,
		"hume/stream_audio_event": event,
	} {
		if err := resultMetadata.Set(key, value); err != nil {
			return nil, err
		}
	}
	result, err := tts.NewResult(audio, resultMetadata)
	if err != nil {
		return nil, err
	}
	responseMetadata := &tts.ResponseMetadata{Model: model}
	if event.RequestID != "" {
		if err := responseMetadata.Set("hume/request_id", event.RequestID); err != nil {
			return nil, err
		}
	}
	return tts.NewResponse(result, responseMetadata)
}
