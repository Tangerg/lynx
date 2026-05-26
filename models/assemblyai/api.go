package assemblyai

import (
	"cmp"
	"context"
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

func (c *APIConfig) validate() error {
	if c == nil {
		return errors.New("assemblyai: config must not be nil")
	}
	if c.APIKey == nil {
		return errors.New("assemblyai: APIKey is required")
	}
	return nil
}

type API struct {
	http *resty.Client
}

func NewAPI(cfg *APIConfig) (*API, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	client := resty.New().
		SetBaseURL(cmp.Or(cfg.BaseURL, DefaultBaseURL)).
		SetHeader("Authorization", cfg.APIKey.Get())
	if cfg.HTTPClient != nil {
		client.SetTransport(cfg.HTTPClient.Transport)
	}

	return &API{http: client}, nil
}

type UploadResponse struct {
	UploadURL string `json:"upload_url"`
}

type TranscriptRequest struct {
	AudioURL              string `json:"audio_url"`
	SpeechModel           string `json:"speech_model,omitempty"`
	LanguageCode          string `json:"language_code,omitempty"`
	LanguageDetection     *bool  `json:"language_detection,omitempty"`
	Punctuate             *bool  `json:"punctuate,omitempty"`
	FormatText            *bool  `json:"format_text,omitempty"`
	SpeakerLabels         *bool  `json:"speaker_labels,omitempty"`
	SentimentAnalysis     *bool  `json:"sentiment_analysis,omitempty"`
	EntityDetection       *bool  `json:"entity_detection,omitempty"`
	IabCategories         *bool  `json:"iab_categories,omitempty"`
	AutoChapters          *bool  `json:"auto_chapters,omitempty"`
	AutoHighlights        *bool  `json:"auto_highlights,omitempty"`
	ContentSafety         *bool  `json:"content_safety,omitempty"`
	Summarization         *bool  `json:"summarization,omitempty"`
	SummaryModel          string `json:"summary_model,omitempty"`
	SummaryType           string `json:"summary_type,omitempty"`
	WebhookURL            string `json:"webhook_url,omitempty"`
	WebhookAuthHeaderName string `json:"webhook_auth_header_name,omitempty"`
}

// TranscriptStatus enumerates the values AssemblyAI puts on
// [TranscriptResponse].Status. Polling treats Completed and Errored as
// terminal; anything else keeps the loop spinning.
type TranscriptStatus = string

const (
	StatusQueued     TranscriptStatus = "queued"
	StatusProcessing TranscriptStatus = "processing"
	StatusCompleted  TranscriptStatus = "completed"
	StatusErrored    TranscriptStatus = "error"
)

type TranscriptResponse struct {
	ID            string  `json:"id"`
	Status        string  `json:"status"`
	Text          string  `json:"text"`
	Confidence    float64 `json:"confidence"`
	AudioDuration int64   `json:"audio_duration"`
	LanguageCode  string  `json:"language_code"`
	Error         string  `json:"error"`
	Utterances    []struct {
		Start      int64   `json:"start"`
		End        int64   `json:"end"`
		Speaker    string  `json:"speaker"`
		Text       string  `json:"text"`
		Confidence float64 `json:"confidence"`
	} `json:"utterances"`
	Words []struct {
		Text       string  `json:"text"`
		Start      int64   `json:"start"`
		End        int64   `json:"end"`
		Confidence float64 `json:"confidence"`
		Speaker    string  `json:"speaker"`
	} `json:"words"`
}

func (a *API) Upload(ctx context.Context, audio []byte) (*UploadResponse, error) {
	if len(audio) == 0 {
		return nil, errors.New("assemblyai: request must not be nil")
	}

	var out UploadResponse
	resp, err := a.http.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/octet-stream").
		SetBody(audio).
		SetResult(&out).
		Post("/upload")
	if err != nil {
		return nil, fmt.Errorf("assemblyai: upload failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("assemblyai: upload http %d: %s", resp.StatusCode(), resp.String())
	}
	return &out, nil
}

func (a *API) CreateTranscript(ctx context.Context, req *TranscriptRequest) (*TranscriptResponse, error) {
	if req == nil {
		return nil, errors.New("assemblyai: request must not be nil")
	}

	var out TranscriptResponse
	resp, err := a.http.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(req).
		SetResult(&out).
		Post("/transcript")
	if err != nil {
		return nil, fmt.Errorf("assemblyai: create transcript failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("assemblyai: http %d: %s", resp.StatusCode(), resp.String())
	}
	return &out, nil
}

func (a *API) Get(ctx context.Context, id string) (*TranscriptResponse, error) {
	var out TranscriptResponse
	resp, err := a.http.R().
		SetContext(ctx).
		SetResult(&out).
		Get("/transcript/" + id)
	if err != nil {
		return nil, fmt.Errorf("assemblyai: get transcript failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("assemblyai: http %d: %s", resp.StatusCode(), resp.String())
	}
	return &out, nil
}
