package revai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/audio/transcription"
	"github.com/Tangerg/lynx/models/internal/options"
)

type AudioTranscriptionModelConfig struct {
	APIKey         model.APIKey
	DefaultOptions *transcription.Options
	BaseURL        string
	HTTPClient     *http.Client
	PollInterval   time.Duration
	PollTimeout    time.Duration
}

func (c AudioTranscriptionModelConfig) Validate() error {
	if c.APIKey == nil {
		return errors.New("revai: APIKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("revai: DefaultOptions is required")
	}
	return nil
}

var _ transcription.Model = (*AudioTranscriptionModel)(nil)

// AudioTranscriptionModel wraps Rev AI's async transcription flow.
// Rev is async-only: Call submits the audio, polls /jobs/{id} until
// "transcribed", then fetches the plain-text transcript.
//
// Diarization, custom vocabularies, profanity filtering, language
// hints and transcriber selection (machine vs human) all live on the
// Extra-threaded [JobOptions].
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
	api, err := NewAPI(APIConfig{APIKey: cfg.APIKey, BaseURL: cfg.BaseURL, HTTPClient: cfg.HTTPClient})
	if err != nil {
		return nil, err
	}
	pi := cfg.PollInterval
	if pi <= 0 {
		pi = DefaultPollInterval
	}
	pt := cfg.PollTimeout
	if pt <= 0 {
		pt = DefaultPollTimeout
	}
	return &AudioTranscriptionModel{api: api, defaultOptions: cfg.DefaultOptions, pollInterval: pi, pollTimeout: pt}, nil
}

func (a *AudioTranscriptionModel) Call(ctx context.Context, req *transcription.Request) (*transcription.Response, error) {
	mergedOpts, err := transcription.MergeOptions(a.defaultOptions, req.Options)
	if err != nil {
		return nil, err
	}

	jobOpts := options.GetParams[JobOptions](mergedOpts, OptionsKey)
	if jobOpts.Language == "" && mergedOpts.Language != "" {
		jobOpts.Language = mergedOpts.Language
	}

	var job *Job
	if jobOpts.MediaURL != "" {
		job, err = a.api.SubmitURL(ctx, *jobOpts)
	} else {
		audio, audioErr := req.Audio.Bytes()
		if audioErr != nil {
			return nil, audioErr
		}
		job, err = a.api.Upload(ctx, audio, *jobOpts)
	}
	if err != nil {
		return nil, err
	}

	final, err := a.pollUntilDone(ctx, job.ID)
	if err != nil {
		return nil, err
	}

	text, err := a.api.GetTranscriptText(ctx, final.ID)
	if err != nil {
		return nil, err
	}

	resultMeta := &transcription.ResultMetadata{}
	if final.Language != "" {
		resultMeta.Set("language", final.Language)
	}
	if final.DurationSeconds > 0 {
		resultMeta.Set("duration_seconds", final.DurationSeconds)
	}

	result, err := transcription.NewResult(text, resultMeta)
	if err != nil {
		return nil, err
	}

	meta := &transcription.ResponseMetadata{}
	meta.Set("job_id", final.ID)
	return transcription.NewResponse(result, meta)
}

func (a *AudioTranscriptionModel) pollUntilDone(ctx context.Context, id string) (*Job, error) {
	deadline, cancel := context.WithTimeout(ctx, a.pollTimeout)
	defer cancel()
	ticker := time.NewTicker(a.pollInterval)
	defer ticker.Stop()
	for {
		resp, err := a.api.GetJob(deadline, id)
		if err != nil {
			return nil, err
		}
		switch resp.Status {
		case "transcribed":
			return resp, nil
		case "failed":
			return nil, fmt.Errorf("revai: transcription failed: %s", resp.FailureReason)
		}
		select {
		case <-deadline.Done():
			return nil, deadline.Err()
		case <-ticker.C:
		}
	}
}

func (a *AudioTranscriptionModel) DefaultOptions() transcription.Options {
	return *a.defaultOptions
}

func (a *AudioTranscriptionModel) Metadata() transcription.ModelMetadata {
	return transcription.ModelMetadata{Provider: Provider}
}
