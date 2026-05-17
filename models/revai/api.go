package revai

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-resty/resty/v2"

	"github.com/Tangerg/lynx/core/model"
)

type ApiConfig struct {
	ApiKey     model.ApiKey
	BaseURL    string
	HTTPClient *http.Client
}

func (c *ApiConfig) validate() error {
	if c == nil {
		return errors.New("revai: config must not be nil")
	}
	if c.ApiKey == nil {
		return errors.New("revai: ApiKey is required")
	}
	return nil
}

type Api struct {
	http *resty.Client
}

func NewApi(cfg *ApiConfig) (*Api, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	client := resty.New().
		SetBaseURL(cmp.Or(cfg.BaseURL, DefaultBaseURL)).
		SetAuthToken(cfg.ApiKey.Get())
	if cfg.HTTPClient != nil {
		client.SetTransport(cfg.HTTPClient.Transport)
	}
	return &Api{http: client}, nil
}

// JobOptions mirrors the JSON the multipart "options" field carries
// when submitting a transcription job. See
// https://docs.rev.ai/api/asynchronous/reference/.
type JobOptions struct {
	MediaURL             string         `json:"media_url,omitempty"`
	SourceConfig         map[string]any `json:"source_config,omitzero"`
	Metadata             string         `json:"metadata,omitempty"`
	CallbackURL          string         `json:"callback_url,omitempty"`
	NotificationConfig   map[string]any `json:"notification_config,omitzero"`
	SkipDiarization      bool           `json:"skip_diarization,omitzero"`
	SkipPunctuation      bool           `json:"skip_punctuation,omitzero"`
	RemoveDisfluencies   bool           `json:"remove_disfluencies,omitzero"`
	RemoveAtmospherics   bool           `json:"remove_atmospherics,omitzero"`
	FilterProfanity      bool           `json:"filter_profanity,omitzero"`
	SpeakerChannelsCount int            `json:"speaker_channels_count,omitempty"`
	Speakers             map[string]any `json:"speakers,omitzero"`
	DiarizationType      string         `json:"diarization_type,omitempty"`
	CustomVocabularyID   string         `json:"custom_vocabulary_id,omitempty"`
	CustomVocabularies   []any          `json:"custom_vocabularies,omitzero"`
	Language             string         `json:"language,omitempty"`
	TranscriberType      string         `json:"transcriber,omitempty"` // "machine_v2" / "human"
	VerbatimMode         bool           `json:"verbatim,omitzero"`
	RushMode             bool           `json:"rush,omitzero"`
	TestMode             bool           `json:"test_mode,omitzero"`
	SegmentsToTranscribe []any          `json:"segments_to_transcribe,omitzero"`
}

// Job is the body Rev returns for /jobs (and the poll body for
// /jobs/{id}). Status moves through "in_progress" / "transcribed" /
// "failed".
type Job struct {
	ID              string  `json:"id"`
	Status          string  `json:"status"`
	CreatedOn       string  `json:"created_on"`
	CompletedOn     string  `json:"completed_on"`
	FailureReason   string  `json:"failure_detail"`
	DurationSeconds float64 `json:"duration_seconds"`
	Language        string  `json:"language"`
}

// Transcript is the simplified transcript shape (text/plain rendering).
// Rev's /jobs/{id}/transcript returns either text or JSON; we ask for
// text by setting Accept: text/plain.
type Transcript = string

// SubmitURL queues a job pointing at media_url. Use Upload when the
// caller has bytes instead.
func (a *Api) SubmitURL(ctx context.Context, opts *JobOptions) (*Job, error) {
	if opts == nil {
		return nil, errors.New("revai: request must not be nil")
	}
	var out Job
	resp, err := a.http.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(opts).
		SetResult(&out).
		Post("/jobs")
	if err != nil {
		return nil, fmt.Errorf("revai: submit failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("revai: http %d: %s", resp.StatusCode(), resp.String())
	}
	return &out, nil
}

// Upload submits a job with the audio bytes as the multipart "media"
// field plus the options as a JSON "options" field.
func (a *Api) Upload(ctx context.Context, audio []byte, opts *JobOptions) (*Job, error) {
	if len(audio) == 0 {
		return nil, errors.New("revai: request must not be nil")
	}
	if opts == nil {
		opts = &JobOptions{}
	}
	optsJSON, _ := json.Marshal(opts)

	var out Job
	resp, err := a.http.R().
		SetContext(ctx).
		SetFileReader("media", "audio", bytes.NewReader(audio)).
		SetMultipartField("options", "", "application/json", bytes.NewReader(optsJSON)).
		SetResult(&out).
		Post("/jobs")
	if err != nil {
		return nil, fmt.Errorf("revai: upload failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("revai: http %d: %s", resp.StatusCode(), resp.String())
	}
	return &out, nil
}

func (a *Api) GetJob(ctx context.Context, id string) (*Job, error) {
	var out Job
	resp, err := a.http.R().SetContext(ctx).SetResult(&out).Get("/jobs/" + id)
	if err != nil {
		return nil, fmt.Errorf("revai: get job failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("revai: http %d: %s", resp.StatusCode(), resp.String())
	}
	return &out, nil
}

// GetTranscriptText fetches the plain-text transcript for a finished
// job. Rev returns 404 until the job reaches "transcribed".
func (a *Api) GetTranscriptText(ctx context.Context, id string) (string, error) {
	resp, err := a.http.R().
		SetContext(ctx).
		SetHeader("Accept", "text/plain").
		Get("/jobs/" + id + "/transcript")
	if err != nil {
		return "", fmt.Errorf("revai: transcript fetch failed: %w", err)
	}
	if !resp.IsSuccess() {
		return "", fmt.Errorf("revai: http %d: %s", resp.StatusCode(), resp.String())
	}
	return resp.String(), nil
}
