package gladia

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
		return errors.New("gladia: APIKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("gladia: DefaultOptions is required")
	}
	return nil
}

var _ transcription.Model = (*AudioTranscriptionModel)(nil)

// AudioTranscriptionModel wraps Gladia's async transcription flow.
// One Call uploads → creates job → polls until "done". Diarization /
// translation / summarization / NER / subtitles all reach the wire via
// the Extra-threaded [TranscriptionRequest].
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

	apiReq := options.GetParams[TranscriptionRequest](mergedOpts, OptionsKey)
	if apiReq.AudioURL == "" {
		var audio []byte
		audio, err = req.Audio.DataAsBytes()
		if err != nil {
			return nil, err
		}
		var uploaded *UploadResponse
		uploaded, err = a.api.Upload(ctx, audio, req.Audio.MimeType.TypeAndSubType())
		if err != nil {
			return nil, err
		}
		apiReq.AudioURL = uploaded.AudioURL
	}

	job, err := a.api.CreateTranscription(ctx, apiReq)
	if err != nil {
		return nil, err
	}

	final, err := a.pollUntilDone(ctx, job.ID)
	if err != nil {
		return nil, err
	}

	resultMeta := &transcription.ResultMetadata{}
	if len(final.Result.Transcription.Languages) > 0 {
		resultMeta.Set("languages", final.Result.Transcription.Languages)
	}
	if len(final.Result.Transcription.Utterances) > 0 {
		resultMeta.Set("utterances", final.Result.Transcription.Utterances)
	}
	if final.Result.Translation != nil {
		resultMeta.Set("translation", final.Result.Translation)
	}
	if final.Result.Summarization != nil {
		resultMeta.Set("summarization", final.Result.Summarization)
	}

	result, err := transcription.NewResult(final.Result.Transcription.FullTranscript, resultMeta)
	if err != nil {
		return nil, err
	}

	meta := &transcription.ResponseMetadata{}
	meta.Set("transcript_id", final.ID)
	return transcription.NewResponse(result, meta)
}

func (a *AudioTranscriptionModel) pollUntilDone(ctx context.Context, id string) (*TranscriptionResult, error) {
	deadline, cancel := context.WithTimeout(ctx, a.pollTimeout)
	defer cancel()
	ticker := time.NewTicker(a.pollInterval)
	defer ticker.Stop()
	for {
		resp, err := a.api.GetTranscription(deadline, id)
		if err != nil {
			return nil, err
		}
		switch resp.Status {
		case "done":
			return resp, nil
		case "error":
			return nil, fmt.Errorf("gladia: transcription failed: %s", resp.ErrorCode)
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
