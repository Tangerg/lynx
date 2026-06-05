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

type APIConfig struct {
	APIKey     model.APIKey
	BaseURL    string
	HTTPClient *http.Client
}

func (c APIConfig) Validate() error {
	if c.APIKey == nil {
		return errors.New("revai: APIKey is required")
	}
	return nil
}

type API struct {
	http *resty.Client
}

func NewAPI(cfg APIConfig) (*API, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	client := resty.New().
		SetBaseURL(cmp.Or(cfg.BaseURL, DefaultBaseURL)).
		SetAuthToken(cfg.APIKey.Get())
	if cfg.HTTPClient != nil {
		client.SetTransport(cfg.HTTPClient.Transport)
	}
	return &API{http: client}, nil
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

// SubmitURL queues a job pointing at media_url. Use Upload when the
// caller has bytes instead.
func (a *API) SubmitURL(ctx context.Context, opts JobOptions) (*Job, error) {
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
func (a *API) Upload(ctx context.Context, audio []byte, opts JobOptions) (*Job, error) {
	if len(audio) == 0 {
		return nil, errors.New("revai: request must not be nil")
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

func (a *API) GetJob(ctx context.Context, id string) (*Job, error) {
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
func (a *API) GetTranscriptText(ctx context.Context, id string) (string, error) {
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
