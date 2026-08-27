package replicate

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	tts "github.com/Tangerg/scope/core/speech"
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

func (a AudioTTSModelConfig) Validate() error {
	if a.APIKey == "" {
		return errors.New("replicate: APIKey is required")
	}
	if a.DefaultOptions.Model == "" {
		return errors.New("replicate: DefaultOptions.Model is required")
	}
	if err := a.DefaultOptions.Validate(); err != nil {
		return err
	}
	if err := a.InputSchema.Validate(); err != nil {
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

func (s SpeechInputSchema) validateOptions(options tts.Options) error {
	unsupported := make([]string, 0, 3)
	if options.OutputFormat != "" {
		unsupported = append(unsupported, "output_format")
	}
	if options.Speed != 0 && s.SpeedKey == "" {
		unsupported = append(unsupported, "speed")
	}
	if options.Voice != "" && s.VoiceKey == "" {
		unsupported = append(unsupported, "voice")
	}
	if len(unsupported) == 0 {
		return nil
	}
	return fmt.Errorf("replicate: speech: unsupported options: %s", strings.Join(unsupported, ", "))
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
	api            *api
	model          string
	inputSchema    SpeechInputSchema
	defaultOptions tts.Options
	pollInterval   time.Duration
	pollTimeout    time.Duration
}

func NewAudioTTSModel(config AudioTTSModelConfig) (*AudioTTSModel, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	api, err := newAPI(apiConfig{
		APIKey:     config.APIKey,
		BaseURL:    config.BaseURL,
		HTTPClient: config.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	pi := config.PollInterval
	if pi <= 0 {
		pi = time.Duration(DefaultPollIntervalSeconds) * time.Second
	}
	pt := config.PollTimeout
	if pt <= 0 {
		pt = time.Duration(DefaultTTSPollTimeoutSeconds) * time.Second
	}
	return &AudioTTSModel{
		api:            api,
		model:          config.DefaultOptions.Model,
		inputSchema:    config.InputSchema,
		defaultOptions: config.DefaultOptions.Clone(),
		pollInterval:   pi,
		pollTimeout:    pt,
	}, nil
}

func (a *AudioTTSModel) Call(ctx context.Context, req *tts.Request) (*tts.Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	effectiveOptions, err := a.defaultOptions.Resolve(req.Options)
	if err != nil {
		return nil, err
	}
	if effectiveOptions.Model != a.model {
		return nil, fmt.Errorf("replicate: speech: model override %q does not match bound schema for %q", effectiveOptions.Model, a.model)
	}
	if validateOptionsErr := a.inputSchema.validateOptions(effectiveOptions); validateOptionsErr != nil {
		return nil, validateOptionsErr
	}

	apiReqValue, _, err := effectiveOptions.Extensions.Decode[predictionRequest](SpeechRequestExtensionKey)

	apiReq := &apiReqValue
	if err != nil {
		return nil, err
	}
	if apiReq.Input == nil {
		apiReq.Input = map[string]any{}
	}

	apiReq.Input[a.inputSchema.TextKey] = req.Text
	if effectiveOptions.Voice != "" {
		apiReq.Input[a.inputSchema.VoiceKey] = effectiveOptions.Voice
	}
	if effectiveOptions.Speed > 0 {
		apiReq.Input[a.inputSchema.SpeedKey] = effectiveOptions.Speed
	}
	if a.inputSchema.VoiceRequired {
		voice, exists := apiReq.Input[a.inputSchema.VoiceKey]
		if !exists || voice == nil || voice == "" {
			return nil, fmt.Errorf("replicate: speech: model %q requires input %q", a.model, a.inputSchema.VoiceKey)
		}
	}

	submit, err := a.api.createPrediction(ctx, effectiveOptions.Model, apiReq)
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

	audio, contentType, err := a.api.downloadOutput(ctx, url)
	if err != nil {
		return nil, err
	}

	outputMetadata := &tts.OutputMetadata{}
	if contentType != "" {
		if setErr := outputMetadata.Set("replicate/mime_type", contentType); setErr != nil {
			return nil, setErr
		}
	}
	if final.Metrics.PredictTime > 0 {
		if setErr := outputMetadata.Set("replicate/predict_time_ms", int64(final.Metrics.PredictTime*1000)); setErr != nil {
			return nil, setErr
		}
	}

	output, err := tts.NewOutput(audio, outputMetadata)
	if err != nil {
		return nil, err
	}

	meta := &tts.ResponseMetadata{Model: effectiveOptions.Model}
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
	return tts.NewResponse(output, meta)
}

// pollUntilDone blocks until the prediction reaches a terminal status.
func (a *AudioTTSModel) pollUntilDone(ctx context.Context, id string) (*predictionResponse, error) {
	deadline, cancel := context.WithTimeout(ctx, a.pollTimeout)
	defer cancel()

	ticker := time.NewTicker(a.pollInterval)
	defer ticker.Stop()

	for {
		resp, err := a.api.getPrediction(deadline, id)
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
