package gladia

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
	PollInterval   time.Duration
	PollTimeout    time.Duration
}

func (c AudioTranscriptionModelConfig) Validate() error {
	if c.APIKey == "" {
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
	if err := req.Validate(); err != nil {
		return nil, err
	}
	mergedOpts, err := a.defaultOptions.Merged(req.Options)
	if err != nil {
		return nil, err
	}
	if err := options.RejectUnsupported("gladia: transcription", map[string]bool{
		"language": mergedOpts.Language != "",
	}); err != nil {
		return nil, err
	}

	apiReq, err := options.GetParams[TranscriptionRequest](mergedOpts.Extra, OptionsKey)
	if err != nil {
		return nil, err
	}
	if apiReq.AudioURL == "" {
		var audio []byte
		audio, err = req.Audio.Bytes()
		if err != nil {
			return nil, err
		}
		var uploaded *UploadResponse
		uploaded, err = a.api.Upload(ctx, audio, req.Audio.MIME)
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
		if err := resultMeta.Set("languages", final.Result.Transcription.Languages); err != nil {
			return nil, err
		}
	}
	if len(final.Result.Transcription.Utterances) > 0 {
		if err := resultMeta.Set("utterances", final.Result.Transcription.Utterances); err != nil {
			return nil, err
		}
	}
	if final.Result.Translation != nil {
		if err := resultMeta.Set("translation", final.Result.Translation); err != nil {
			return nil, err
		}
	}
	if final.Result.Summarization != nil {
		if err := resultMeta.Set("summarization", final.Result.Summarization); err != nil {
			return nil, err
		}
	}

	result, err := transcription.NewResult(final.Result.Transcription.FullTranscript, resultMeta)
	if err != nil {
		return nil, err
	}

	meta := &transcription.ResponseMetadata{}
	if err := meta.Set("transcript_id", final.ID); err != nil {
		return nil, err
	}
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
