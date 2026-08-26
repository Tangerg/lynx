package gladia

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Tangerg/lynx/core/transcription"
)

type AudioTranscriptionModelConfig struct {
	APIKey         string
	DefaultOptions transcription.Options
	BaseURL        string
	HTTPClient     *http.Client
	PollInterval   time.Duration
	PollTimeout    time.Duration
}

func (a AudioTranscriptionModelConfig) Validate() error {
	if a.APIKey == "" {
		return errors.New("gladia: APIKey is required")
	}
	if a.DefaultOptions.Model == "" {
		return errors.New("gladia: DefaultOptions.Model is required")
	}
	if _, err := a.DefaultOptions.Merged(); err != nil {
		return err
	}
	return nil
}

var _ transcription.Model = (*AudioTranscriptionModel)(nil)

// AudioTranscriptionModel wraps Gladia's async transcription flow.
// One Call uploads → creates job → polls until "done". Diarization /
// translation / summarization / NER / subtitles all reach the wire via
// the extension-threaded [TranscriptionRequest].
type AudioTranscriptionModel struct {
	api            *api
	defaultOptions transcription.Options
	pollInterval   time.Duration
	pollTimeout    time.Duration
}

func NewAudioTranscriptionModel(cfg AudioTranscriptionModelConfig) (*AudioTranscriptionModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	api, err := newAPI(apiConfig{APIKey: cfg.APIKey, BaseURL: cfg.BaseURL, HTTPClient: cfg.HTTPClient})
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
	return &AudioTranscriptionModel{api: api, defaultOptions: cfg.DefaultOptions.Clone(), pollInterval: pi, pollTimeout: pt}, nil
}

func (a *AudioTranscriptionModel) Call(ctx context.Context, req *transcription.Request) (*transcription.Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	mergedOpts, err := a.defaultOptions.Merged(req.Options)
	if err != nil {
		return nil, err
	}
	apiReqValue, _, err := mergedOpts.Extensions.Decode[transcriptionRequest](RequestExtensionKey)
	apiReq := &apiReqValue
	if err != nil {
		return nil, err
	}
	apiReq.Model = mergedOpts.Model
	if mergedOpts.Language != "" {
		if apiReq.LanguageConfig == nil {
			apiReq.LanguageConfig = &languageConfig{}
		}
		apiReq.LanguageConfig.Languages = []string{mergedOpts.Language}
	}
	if validateTranscriptionRequestErr := validateTranscriptionRequest(apiReq); validateTranscriptionRequestErr != nil {
		return nil, validateTranscriptionRequestErr
	}
	if apiReq.AudioURL == "" {
		var audio []byte
		audio, err = req.Audio.Bytes()
		if err != nil {
			return nil, err
		}
		var uploaded *uploadResponse
		uploaded, err = a.api.upload(ctx, audio, req.Audio.MIME)
		if err != nil {
			return nil, err
		}
		apiReq.AudioURL = uploaded.AudioURL
	}

	job, err := a.api.createTranscription(ctx, apiReq)
	if err != nil {
		return nil, err
	}

	final, err := a.pollUntilDone(ctx, job.ID)
	if err != nil {
		return nil, err
	}

	outputMetadata := &transcription.OutputMetadata{}
	if len(final.Result.Transcription.Languages) > 0 {
		if setErr := outputMetadata.Set("gladia/languages", final.Result.Transcription.Languages); setErr != nil {
			return nil, setErr
		}
	}
	if len(final.Result.Transcription.Utterances) > 0 {
		if setErr := outputMetadata.Set("gladia/utterances", final.Result.Transcription.Utterances); setErr != nil {
			return nil, setErr
		}
	}
	if final.Result.Translation != nil {
		if setErr := outputMetadata.Set("gladia/translation", final.Result.Translation); setErr != nil {
			return nil, setErr
		}
	}
	if final.Result.Summarization != nil {
		if setErr := outputMetadata.Set("gladia/summarization", final.Result.Summarization); setErr != nil {
			return nil, setErr
		}
	}

	output, err := transcription.NewOutput(final.Result.Transcription.FullTranscript, outputMetadata)
	if err != nil {
		return nil, err
	}

	meta := &transcription.ResponseMetadata{Model: apiReq.Model}
	if err := meta.Set("gladia/transcript_id", final.ID); err != nil {
		return nil, err
	}
	if err := meta.Set(ResponseExtensionKey, final.Raw); err != nil {
		return nil, err
	}
	return transcription.NewResponse(output, meta)
}

func validateTranscriptionRequest(req *transcriptionRequest) error {
	if req.Model != ModelSolaria3 && req.Model != ModelSolaria1 {
		return fmt.Errorf("gladia: transcription model must be %q or %q, got %q", ModelSolaria3, ModelSolaria1, req.Model)
	}
	if req.Model == ModelSolaria3 {
		if req.LanguageConfig == nil || len(req.LanguageConfig.Languages) != 1 {
			return errors.New("gladia: solaria-3 requires exactly one language_config.languages entry")
		}
		if req.LanguageConfig.CodeSwitching != nil && *req.LanguageConfig.CodeSwitching {
			return errors.New("gladia: solaria-3 does not support language code switching")
		}
	}
	return nil
}

func (a *AudioTranscriptionModel) pollUntilDone(ctx context.Context, id string) (*transcriptionResult, error) {
	deadline, cancel := context.WithTimeout(ctx, a.pollTimeout)
	defer cancel()
	ticker := time.NewTicker(a.pollInterval)
	defer ticker.Stop()
	for {
		resp, err := a.api.getTranscription(deadline, id)
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
