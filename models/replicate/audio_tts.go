package replicate

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	tts "github.com/Tangerg/lynx/core/speech"
	"github.com/Tangerg/lynx/models/internal/options"
)

type AudioTTSModelConfig struct {
	APIKey         string
	DefaultOptions tts.Options
	InputSchema    SpeechInputSchema
	BaseURL        string
	HTTPClient     *http.Client

	// PollInterval / PollTimeout configure the synchronous wrapper
	// around Replicate's async generation. Zero values fall back to
	// the package defaults; community TTS jobs can include cold-start
	// latency, so PollTimeout defaults higher than image.
	PollInterval time.Duration
	PollTimeout  time.Duration
}

func (c AudioTTSModelConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("replicate: APIKey is required")
	}
	if c.DefaultOptions.Model == "" {
		return errors.New("replicate: DefaultOptions.Model is required")
	}
	if _, err := c.DefaultOptions.Merged(); err != nil {
		return err
	}
	if err := c.InputSchema.Validate(); err != nil {
		return err
	}
	return nil
}

// SpeechInputSchema explicitly binds Core speech fields to one Replicate
// model version's OpenAPI schema.
type SpeechInputSchema struct {
	TextKey       string
	VoiceKey      string
	SpeedKey      string
	VoiceRequired bool
	OutputKind    FileOutputKind
}

func (s SpeechInputSchema) Validate() error {
	if s.TextKey == "" {
		return errors.New("replicate: SpeechInputSchema.TextKey is required")
	}
	if s.VoiceRequired && s.VoiceKey == "" {
		return errors.New("replicate: SpeechInputSchema.VoiceRequired requires VoiceKey")
	}
	if s.OutputKind != FileOutputURI && s.OutputKind != FileOutputURIList {
		return fmt.Errorf("replicate: SpeechInputSchema.OutputKind must be %q or %q", FileOutputURI, FileOutputURIList)
	}
	keys := []string{s.TextKey, s.VoiceKey, s.SpeedKey}
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if key == "" {
			continue
		}
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("replicate: SpeechInputSchema maps multiple fields to input key %q", key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

// XTTSV2SpeechInputSchema returns the official schema binding for the pinned
// [ModelXTTSV2] version.
func XTTSV2SpeechInputSchema() SpeechInputSchema {
	return SpeechInputSchema{
		TextKey:       "text",
		VoiceKey:      "speaker",
		VoiceRequired: true,
		OutputKind:    FileOutputURI,
	}
}

var _ tts.Model = (*AudioTTSModel)(nil)

// AudioTTSModel wraps one explicitly bound Replicate TTS model version. It
// never infers fields from a model name: community models have independent,
// versioned schemas and must be constructed with the matching binding.
type AudioTTSModel struct {
	api            *API
	model          string
	inputSchema    SpeechInputSchema
	defaultOptions tts.Options
	pollInterval   time.Duration
	pollTimeout    time.Duration
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
	return &AudioTTSModel{
		api:            api,
		model:          cfg.DefaultOptions.Model,
		inputSchema:    cfg.InputSchema,
		defaultOptions: cfg.DefaultOptions.Clone(),
		pollInterval:   pi,
		pollTimeout:    pt,
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
	if mergedOpts.Model != a.model {
		return nil, fmt.Errorf("replicate: speech: model override %q does not match bound schema for %q", mergedOpts.Model, a.model)
	}
	if err := options.RejectUnsupported("replicate: speech", map[string]bool{
		"output_format": mergedOpts.OutputFormat != "",
		"speed":         mergedOpts.Speed != 0 && a.inputSchema.SpeedKey == "",
		"voice":         mergedOpts.Voice != "" && a.inputSchema.VoiceKey == "",
	}); err != nil {
		return nil, err
	}

	apiReq, err := options.GetParams[PredictionRequest](mergedOpts.Extensions, SpeechRequestExtensionKey)
	if err != nil {
		return nil, err
	}
	if apiReq.Input == nil {
		apiReq.Input = map[string]any{}
	}

	apiReq.Input[a.inputSchema.TextKey] = req.Text
	if mergedOpts.Voice != "" {
		apiReq.Input[a.inputSchema.VoiceKey] = mergedOpts.Voice
	}
	if mergedOpts.Speed > 0 {
		apiReq.Input[a.inputSchema.SpeedKey] = mergedOpts.Speed
	}
	if a.inputSchema.VoiceRequired {
		voice, exists := apiReq.Input[a.inputSchema.VoiceKey]
		if !exists || voice == nil || voice == "" {
			return nil, fmt.Errorf("replicate: speech: model %q requires input %q", a.model, a.inputSchema.VoiceKey)
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

	url, err := firstAudioURL(final.Output, a.inputSchema.OutputKind)
	if err != nil {
		return nil, err
	}

	audio, contentType, err := a.api.DownloadOutput(ctx, url)
	if err != nil {
		return nil, err
	}

	resultMeta := &tts.ResultMetadata{}
	if contentType != "" {
		if err := resultMeta.Set("replicate/mime_type", contentType); err != nil {
			return nil, err
		}
	}
	if final.Metrics.PredictTime > 0 {
		if err := resultMeta.Set("replicate/predict_time_ms", int64(final.Metrics.PredictTime*1000)); err != nil {
			return nil, err
		}
	}

	result, err := tts.NewResult(audio, resultMeta)
	if err != nil {
		return nil, err
	}

	meta := &tts.ResponseMetadata{Model: mergedOpts.Model}
	if final.CreatedAt != "" {
		createdAt, err := time.Parse(time.RFC3339Nano, final.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("replicate: invalid prediction created_at %q: %w", final.CreatedAt, err)
		}
		meta.Created = createdAt.Unix()
	}
	if err := meta.Set("replicate/prediction_id", final.ID); err != nil {
		return nil, err
	}
	if final.Version != "" {
		if err := meta.Set("replicate/version", final.Version); err != nil {
			return nil, err
		}
	}
	if err := meta.Set("replicate/audio_url", url); err != nil {
		return nil, err
	}
	if err := meta.Set(SpeechResponseExtensionKey, final); err != nil {
		return nil, err
	}
	return tts.NewResponse(result, meta)
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

func firstAudioURL(out any, kind FileOutputKind) (string, error) {
	if out == nil {
		return "", errors.New("replicate: speech output is null")
	}
	switch kind {
	case FileOutputURI:
		value, ok := out.(string)
		if !ok || value == "" {
			return "", fmt.Errorf("replicate: speech output must be a non-empty URI, got %T", out)
		}
		return value, nil
	case FileOutputURIList:
		values, ok := out.([]any)
		if !ok || len(values) != 1 {
			return "", fmt.Errorf("replicate: speech output must be a one-element URI array, got %T", out)
		}
		value, ok := values[0].(string)
		if !ok || value == "" {
			return "", fmt.Errorf("replicate: speech output[0] must be a non-empty URI, got %T", values[0])
		}
		return value, nil
	default:
		return "", fmt.Errorf("replicate: unsupported speech output schema %q", kind)
	}
}
