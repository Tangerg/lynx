package assemblyai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Tangerg/lynx/core/transcription"
	"github.com/Tangerg/lynx/models/internal/options"
)

type AudioTranscriptionModelConfig struct {
	APIKey         string
	DefaultOptions *transcription.Options
	BaseURL        string
	HTTPClient     *http.Client

	// PollInterval / PollTimeout configure the synchronous Call
	// wrapper around AssemblyAI's async job model. Zero values fall
	// back to [DefaultPollInterval] / [DefaultPollTimeout].
	PollInterval time.Duration
	PollTimeout  time.Duration
}

func (c AudioTranscriptionModelConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("assemblyai: APIKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("assemblyai: DefaultOptions is required")
	}
	return nil
}

var _ transcription.Model = (*AudioTranscriptionModel)(nil)

// AudioTranscriptionModel wraps AssemblyAI's async transcription flow
// behind a synchronous [transcription.Model.Call] surface. One Call
// uploads the audio, enqueues a job, and polls until the job reaches a
// terminal state — callers don't see the polling unless their ctx
// cancels or [PollTimeout] elapses.
//
// Speaker labels, sentiment analysis, auto chapters, entity detection
// and the rest of AssemblyAI's analysis features live on
// [TranscriptRequest] and reach the API via the Extra-threaded SDK
// params, see [getOptionsParams].
//
// Audio source: the [transcription.Request].Audio is uploaded by
// bytes; if the audio is large and already hosted somewhere the API
// can reach, callers can override the audio_url by setting it on the
// Extra-threaded TranscriptRequest and the model will skip the
// /upload roundtrip.
type AudioTranscriptionModel struct {
	api            *API
	defaultOptions *transcription.Options
	pollInterval   time.Duration
	pollTimeout    time.Duration
}

func NewAudioTranscriptionModel(cfg AudioTranscriptionModelConfig) (*AudioTranscriptionModel, error) {
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

	pollInterval := cfg.PollInterval
	if pollInterval <= 0 {
		pollInterval = DefaultPollInterval
	}
	pollTimeout := cfg.PollTimeout
	if pollTimeout <= 0 {
		pollTimeout = DefaultPollTimeout
	}

	return &AudioTranscriptionModel{
		api:            api,
		defaultOptions: cfg.DefaultOptions,
		pollInterval:   pollInterval,
		pollTimeout:    pollTimeout,
	}, nil
}

func (a *AudioTranscriptionModel) Call(ctx context.Context, req *transcription.Request) (*transcription.Response, error) {
	mergedOpts, err := transcription.MergeOptions(a.defaultOptions, req.Options)
	if err != nil {
		return nil, err
	}

	apiReq := options.GetParams[TranscriptRequest](mergedOpts, OptionsKey)
	if apiReq.SpeechModel == "" {
		apiReq.SpeechModel = mergedOpts.Model
	}
	if apiReq.LanguageCode == "" && mergedOpts.Language != "" {
		apiReq.LanguageCode = mergedOpts.Language
	}

	// Skip the /upload roundtrip when the caller already gave us a
	// reachable URL via Extra; otherwise upload the bytes.
	if apiReq.AudioURL == "" {
		var audio []byte
		audio, err = req.Audio.Bytes()
		if err != nil {
			return nil, err
		}
		var uploaded *UploadResponse
		uploaded, err = a.api.Upload(ctx, audio)
		if err != nil {
			return nil, err
		}
		apiReq.AudioURL = uploaded.UploadURL
	}

	job, err := a.api.CreateTranscript(ctx, apiReq)
	if err != nil {
		return nil, err
	}

	final, err := a.pollUntilDone(ctx, job.ID)
	if err != nil {
		return nil, err
	}

	return a.buildResponse(final)
}

// pollUntilDone re-fetches the transcript every [pollInterval] until
// it reaches "completed" / "error", or the ctx / pollTimeout deadline
// fires.
func (a *AudioTranscriptionModel) pollUntilDone(ctx context.Context, id string) (*TranscriptResponse, error) {
	deadlineCtx, cancel := context.WithTimeout(ctx, a.pollTimeout)
	defer cancel()

	ticker := time.NewTicker(a.pollInterval)
	defer ticker.Stop()

	// First fetch immediately rather than waiting one tick — short
	// audio often finishes before our first poll.
	for {
		resp, err := a.api.Get(deadlineCtx, id)
		if err != nil {
			return nil, err
		}
		switch resp.Status {
		case StatusCompleted:
			return resp, nil
		case StatusErrored:
			return nil, fmt.Errorf("assemblyai: transcription failed: %s", resp.Error)
		}

		select {
		case <-deadlineCtx.Done():
			return nil, deadlineCtx.Err()
		case <-ticker.C:
		}
	}
}

func (a *AudioTranscriptionModel) buildResponse(apiResp *TranscriptResponse) (*transcription.Response, error) {
	resultMeta := &transcription.ResultMetadata{}
	resultMeta.Set("confidence", apiResp.Confidence)
	if apiResp.LanguageCode != "" {
		resultMeta.Set("language_code", apiResp.LanguageCode)
	}
	if len(apiResp.Utterances) > 0 {
		resultMeta.Set("utterances", apiResp.Utterances)
	}
	if len(apiResp.Words) > 0 {
		resultMeta.Set("words", apiResp.Words)
	}

	result, err := transcription.NewResult(apiResp.Text, resultMeta)
	if err != nil {
		return nil, err
	}

	meta := &transcription.ResponseMetadata{}
	meta.Set("transcript_id", apiResp.ID)
	meta.Set("audio_duration", apiResp.AudioDuration)

	return transcription.NewResponse(result, meta)
}
