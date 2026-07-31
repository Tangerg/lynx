package mistral

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-resty/resty/v2"
)

// API is the authenticated transport for Mistral's native endpoints.
type APIConfig struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

// APIError reports a non-successful Mistral HTTP response.
type APIError struct {
	Operation  string
	StatusCode int
	RequestID  string
	Message    string
}

func (err *APIError) Error() string {
	if err == nil {
		return "mistral: nil API error"
	}
	detail := fmt.Sprintf("mistral: %s returned HTTP %d", err.Operation, err.StatusCode)
	if err.Message != "" {
		detail += ": " + err.Message
	}
	if err.RequestID != "" {
		detail += " (request_id=" + err.RequestID + ")"
	}
	return detail
}

func (c APIConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("mistral: APIKey is required")
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
	client := resty.New()
	if cfg.HTTPClient != nil {
		client = resty.NewWithClient(cfg.HTTPClient)
	}
	client.
		SetBaseURL(cmp.Or(cfg.BaseURL, DefaultBaseURL)).
		SetAuthToken(cfg.APIKey).
		SetHeader("Content-Type", "application/json")
	return &API{http: client}, nil
}

// ModerationRequest mirrors POST /moderations. Mistral's moderation API
// takes a free-form `input` (string or array of strings) plus a model
// id ("mistral-moderation-2603" is current).
type ModerationRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

// ModerationResponse mirrors the response. Mistral's category set is
// custom (sexual, hate_and_discrimination, violence_and_threats,
// dangerous_and_criminal_content, selfharm, health, financial, law,
// pii) — different from OpenAI's, hence the dedicated endpoint.
type ModerationResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Results []struct {
		Categories     map[string]bool    `json:"categories"`
		CategoryScores map[string]float64 `json:"category_scores"`
	} `json:"results"`
}

func (a *API) Moderation(ctx context.Context, req *ModerationRequest) (*ModerationResponse, error) {
	if req == nil {
		return nil, errors.New("mistral: request must not be nil")
	}
	var out ModerationResponse
	resp, err := a.http.R().SetContext(ctx).SetBody(req).SetResult(&out).Post("/moderations")
	if err != nil {
		return nil, fmt.Errorf("mistral: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("mistral: http %d: %s", resp.StatusCode(), resp.String())
	}
	return &out, nil
}

func (a *API) chatCompletion(ctx context.Context, request *chatCompletionRequest) (*chatCompletionResponse, error) {
	if a == nil || a.http == nil {
		return nil, errors.New("mistral: nil API")
	}
	if request == nil {
		return nil, errors.New("mistral: chat completion request must not be nil")
	}
	var result chatCompletionResponse
	response, err := a.http.R().
		SetContext(ctx).
		SetBody(request).
		SetResult(&result).
		Post("/chat/completions")
	if err != nil {
		return nil, fmt.Errorf("mistral: chat completion request: %w", err)
	}
	if !response.IsSuccess() {
		return nil, newAPIError("chat completion", response.StatusCode(), response.Header(), response.Body())
	}
	return &result, nil
}

func (a *API) chatCompletionStream(ctx context.Context, request *chatCompletionRequest) (io.ReadCloser, error) {
	if a == nil || a.http == nil {
		return nil, errors.New("mistral: nil API")
	}
	if request == nil {
		return nil, errors.New("mistral: chat completion request must not be nil")
	}
	response, err := a.http.R().
		SetContext(ctx).
		SetBody(request).
		SetDoNotParseResponse(true).
		Post("/chat/completions")
	if err != nil {
		return nil, fmt.Errorf("mistral: chat completion stream request: %w", err)
	}
	body := response.RawBody()
	if response.IsSuccess() {
		return body, nil
	}
	defer body.Close()
	payload, readErr := io.ReadAll(io.LimitReader(body, 1<<20))
	if readErr != nil {
		return nil, fmt.Errorf("mistral: read chat completion stream error: %w", readErr)
	}
	return nil, newAPIError("chat completion stream", response.StatusCode(), response.Header(), payload)
}

func newAPIError(operation string, statusCode int, header http.Header, body []byte) error {
	requestID := header.Get("request-id")
	if requestID == "" {
		requestID = header.Get("x-request-id")
	}
	message := strings.TrimSpace(string(body))
	var payload struct {
		Message string `json:"message"`
		Detail  any    `json:"detail"`
	}
	if json.Unmarshal(body, &payload) == nil {
		switch {
		case payload.Message != "":
			message = payload.Message
		case payload.Detail != nil:
			if encoded, err := json.Marshal(payload.Detail); err == nil {
				message = string(encoded)
			}
		}
	}
	return &APIError{
		Operation:  operation,
		StatusCode: statusCode,
		RequestID:  requestID,
		Message:    message,
	}
}
