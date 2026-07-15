package replicate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"net/http"
	"strings"
	"time"

	tts "github.com/Tangerg/lynx/core/speech"
	"github.com/Tangerg/lynx/models/internal/options"
)

type AudioTTSModelConfig struct {
	APIKey         string
	DefaultOptions *tts.Options
	BaseURL        string
	HTTPClient     *http.Client

	// PollInterval / PollTimeout configure the synchronous wrapper
	// around Replicate's async generation. Zero values fall back to
	// the package defaults — TTS jobs (especially Tortoise) can take
	// 30s+ so PollTimeout defaults higher than image.
	PollInterval time.Duration
	PollTimeout  time.Duration
}

func (c AudioTTSModelConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("replicate: APIKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("replicate: DefaultOptions is required")
	}
	return nil
}

var _ tts.Model = (*AudioTTSModel)(nil)
var _ tts.Streamer = (*AudioTTSModel)(nil)

// AudioTTSModel wraps Replicate's TTS surface. It targets open-weight
// TTS models that don't ship as commercial APIs — XTTS-v2 (voice
// cloning), Bark (mixed speech / song / sfx), Tortoise-TTS (highest
// quality, slow).
//
// Field mapping. [tts.Request].Text maps to "text" for most models
// or "prompt" for Bark; [tts.Options].Voice maps to "speaker" for
// XTTS, "voice_a" for Tortoise, and "history_prompt" for Bark. To
// stay accurate across the long tail of community models, callers
// should set provider-specific keys directly via the
// Extra-threaded PredictionRequest at [OptionsKey].
type AudioTTSModel struct {
	api            *API
	defaultOptions *tts.Options
	pollInterval   time.Duration
	pollTimeout    time.Duration
	httpClient     *http.Client
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
	pi := cfg.PollInterval
	if pi <= 0 {
		pi = time.Duration(DefaultPollIntervalSeconds) * time.Second
	}
	pt := cfg.PollTimeout
	if pt <= 0 {
		pt = time.Duration(DefaultTTSPollTimeoutSeconds) * time.Second
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = http.DefaultClient
	}
	return &AudioTTSModel{
		api:            api,
		defaultOptions: cfg.DefaultOptions,
		pollInterval:   pi,
		pollTimeout:    pt,
		httpClient:     hc,
	}, nil
}

func (a *AudioTTSModel) Call(ctx context.Context, req *tts.Request) (*tts.Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	mergedOpts, err := a.defaultOptions.Merged(req.Options)
	if err != nil {
		return nil, err
	}
	if err := options.RejectUnsupported("replicate: speech", map[string]bool{
		"output_format": mergedOpts.OutputFormat != "",
	}); err != nil {
		return nil, err
	}

	apiReq, err := options.GetParams[PredictionRequest](mergedOpts.Extra, OptionsKey)
	if err != nil {
		return nil, err
	}
	if apiReq.Input == nil {
		apiReq.Input = map[string]any{}
	}

	textKey, voiceKey := ttsInputKeys(mergedOpts.Model)
	if _, set := apiReq.Input[textKey]; !set {
		apiReq.Input[textKey] = req.Text
	}
	if mergedOpts.Voice != "" {
		if _, set := apiReq.Input[voiceKey]; !set {
			apiReq.Input[voiceKey] = mergedOpts.Voice
		}
	}
	if mergedOpts.Speed > 0 {
		if _, set := apiReq.Input["speed"]; !set {
			apiReq.Input["speed"] = mergedOpts.Speed
		}
	}

	submit, err := a.api.CreatePrediction(ctx, mergedOpts.Model, apiReq)
	if err != nil {
		return nil, err
	}

	final, err := a.pollUntilDone(ctx, submit.ID)
	if err != nil {
		return nil, err
	}

	url, err := firstAudioURL(final.Output)
	if err != nil {
		return nil, err
	}

	audio, contentType, err := a.fetchAudio(ctx, url)
	if err != nil {
		return nil, err
	}

	resultMeta := &tts.ResultMetadata{}
	if contentType != "" {
		if err := resultMeta.Set("mime_type", contentType); err != nil {
			return nil, err
		}
	}
	if final.Metrics.PredictTime > 0 {
		if err := resultMeta.Set("predict_time_ms", int64(final.Metrics.PredictTime*1000)); err != nil {
			return nil, err
		}
	}

	result, err := tts.NewResult(audio, resultMeta)
	if err != nil {
		return nil, err
	}

	meta := &tts.ResponseMetadata{Model: mergedOpts.Model, Created: time.Now().Unix()}
	if err := meta.Set("prediction_id", final.ID); err != nil {
		return nil, err
	}
	if final.Version != "" {
		if err := meta.Set("version", final.Version); err != nil {
			return nil, err
		}
	}
	if err := meta.Set("audio_url", url); err != nil {
		return nil, err
	}
	return tts.NewResponse(result, meta)
}

func (a *AudioTTSModel) Stream(ctx context.Context, req *tts.Request) iter.Seq2[*tts.Response, error] {
	// Replicate has no streaming TTS API — yield the full audio as a
	// single chunk so callers writing against tts.Model.Stream still
	// work, just without incremental playback.
	return func(yield func(*tts.Response, error) bool) {
		resp, err := a.Call(ctx, req)
		if err != nil {
			yield(nil, err)
			return
		}
		yield(resp, nil)
	}
}

// pollUntilDone blocks until the prediction reaches a terminal status.
func (a *AudioTTSModel) pollUntilDone(ctx context.Context, id string) (*PredictionResponse, error) {
	deadline, cancel := context.WithTimeout(ctx, a.pollTimeout)
	defer cancel()

	ticker := time.NewTicker(a.pollInterval)
	defer ticker.Stop()

	for {
		resp, err := a.api.GetPrediction(deadline, id)
		if err != nil {
			return nil, err
		}
		switch resp.Status {
		case "succeeded":
			return resp, nil
		case "failed", "canceled":
			msg := resp.Error
			if msg == "" {
				msg = resp.Status
			}
			return nil, fmt.Errorf("replicate: generation %s: %s", resp.Status, msg)
		}
		select {
		case <-deadline.Done():
			return nil, deadline.Err()
		case <-ticker.C:
		}
	}
}

// fetchAudio downloads the audio bytes from a Replicate-hosted URL
// (replicate.delivery CDN). Returns the bytes plus the response
// Content-Type for downstream metadata.
func (a *AudioTTSModel) fetchAudio(ctx context.Context, url string) ([]byte, string, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("replicate: build audio fetch: %w", err)
	}
	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return nil, "", fmt.Errorf("replicate: fetch audio: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, "", fmt.Errorf("replicate: fetch audio http %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("replicate: read audio: %w", err)
	}
	if len(body) == 0 {
		return nil, "", errors.New("replicate: empty audio response")
	}
	return body, resp.Header.Get("Content-Type"), nil
}

// firstAudioURL extracts the audio URL from a prediction's output.
// Most TTS models return a single string URL; some (Bark variants)
// wrap it in an array.
func firstAudioURL(out any) (string, error) {
	switch v := out.(type) {
	case string:
		if v == "" {
			return "", errors.New("replicate: empty output URL")
		}
		return v, nil
	case []any:
		if len(v) == 0 {
			return "", errors.New("replicate: empty output array")
		}
		s, ok := v[0].(string)
		if !ok {
			return "", fmt.Errorf("replicate: unsupported output element type %T", v[0])
		}
		return s, nil
	case map[string]any:
		// Bark on Replicate returns { "audio_out": "url" }.
		for _, key := range []string{"audio_out", "audio", "url"} {
			if val, ok := v[key]; ok {
				if s, isStr := val.(string); isStr && s != "" {
					return s, nil
				}
			}
		}
		return "", errors.New("replicate: no audio URL in map output")
	case nil:
		return "", errors.New("replicate: output is null")
	default:
		return "", fmt.Errorf("replicate: unsupported output type %T", out)
	}
}

// ttsInputKeys returns (textKey, voiceKey) for the given Replicate
// model id. Bark uses "prompt" + "history_prompt"; Tortoise uses
// "text" + "voice_a"; XTTS and most others use "text" + "speaker".
func ttsInputKeys(modelID string) (textKey, voiceKey string) {
	_, name, _ := parseModelID(modelID)
	switch {
	case strings.Contains(name, "bark"):
		return "prompt", "history_prompt"
	case strings.Contains(name, "tortoise"):
		return "text", "voice_a"
	default:
		return "text", "speaker"
	}
}
