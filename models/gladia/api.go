package gladia

import (
	"bytes"
	"cmp"
	"context"
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
		return errors.New("gladia: config must not be nil")
	}
	if c.ApiKey == nil {
		return errors.New("gladia: ApiKey is required")
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
		SetHeader("x-gladia-key", cfg.ApiKey.Get())
	if cfg.HTTPClient != nil {
		client.SetTransport(cfg.HTTPClient.Transport)
	}
	return &Api{http: client}, nil
}

type UploadResponse struct {
	AudioURL      string `json:"audio_url"`
	AudioMetadata struct {
		ID            string  `json:"id"`
		AudioDuration float64 `json:"audio_duration"`
		Channels      int     `json:"channels"`
	} `json:"audio_metadata"`
}

type TranscriptionRequest struct {
	AudioURL               string         `json:"audio_url"`
	DetectLanguage         *bool          `json:"detect_language,omitempty"`
	EnableCodeSwitching    *bool          `json:"enable_code_switching,omitempty"`
	Diarization            *bool          `json:"diarization,omitempty"`
	DiarizationConfig      map[string]any `json:"diarization_config,omitzero"`
	Translation            *bool          `json:"translation,omitempty"`
	TranslationConfig      map[string]any `json:"translation_config,omitzero"`
	Summarization          *bool          `json:"summarization,omitempty"`
	SummarizationConfig    map[string]any `json:"summarization_config,omitzero"`
	NamedEntityRecognition *bool          `json:"named_entity_recognition,omitempty"`
	SentimentAnalysis      *bool          `json:"sentiment_analysis,omitempty"`
	Subtitles              *bool          `json:"subtitles,omitempty"`
	SubtitlesConfig        map[string]any `json:"subtitles_config,omitzero"`
	Punctuation            *bool          `json:"punctuation,omitempty"`
	CustomVocabulary       []any          `json:"custom_vocabulary,omitzero"`
}

type TranscriptionCreateResponse struct {
	ID        string `json:"id"`
	ResultURL string `json:"result_url"`
}

// TranscriptionResult is the body of GET /pre-recorded/{id}. Status moves
// through "queued" / "processing" / "done" / "error".
type TranscriptionResult struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Result struct {
		Transcription struct {
			FullTranscript string   `json:"full_transcript"`
			Languages      []string `json:"languages,omitzero"`
			Utterances     []any    `json:"utterances,omitzero"`
		} `json:"transcription"`
		Translation   any `json:"translation,omitempty"`
		Summarization any `json:"summarization,omitempty"`
	} `json:"result"`
	ErrorCode string `json:"error_code,omitempty"`
}

// Upload posts raw audio bytes to /upload, returning a Gladia-hosted
// URL the caller passes to /pre-recorded.
func (a *Api) Upload(ctx context.Context, audio []byte, mimeType string) (*UploadResponse, error) {
	if len(audio) == 0 {
		return nil, errors.New("gladia: request must not be nil")
	}
	var out UploadResponse
	resp, err := a.http.R().
		SetContext(ctx).
		SetFileReader("audio", "audio", bytes.NewReader(audio)).
		SetResult(&out).
		Post("/upload")
	if err != nil {
		return nil, fmt.Errorf("gladia: upload failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("gladia: upload http %d: %s", resp.StatusCode(), resp.String())
	}
	return &out, nil
}

func (a *Api) CreateTranscription(ctx context.Context, req *TranscriptionRequest) (*TranscriptionCreateResponse, error) {
	if req == nil {
		return nil, errors.New("gladia: request must not be nil")
	}
	var out TranscriptionCreateResponse
	resp, err := a.http.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(req).
		SetResult(&out).
		Post("/pre-recorded")
	if err != nil {
		return nil, fmt.Errorf("gladia: create failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("gladia: http %d: %s", resp.StatusCode(), resp.String())
	}
	return &out, nil
}

func (a *Api) GetTranscription(ctx context.Context, id string) (*TranscriptionResult, error) {
	var out TranscriptionResult
	resp, err := a.http.R().SetContext(ctx).SetResult(&out).Get("/pre-recorded/" + id)
	if err != nil {
		return nil, fmt.Errorf("gladia: poll failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("gladia: http %d: %s", resp.StatusCode(), resp.String())
	}
	return &out, nil
}
