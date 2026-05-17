package midjourney

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-resty/resty/v2"

	"github.com/Tangerg/lynx/core/model"
)

// ApiConfig is intentionally proxy-shaped: BaseURL is required because
// Midjourney has no official REST endpoint. SubmitPath / FetchPath let
// callers point at proxies that use non-default paths.
type ApiConfig struct {
	ApiKey     model.ApiKey
	BaseURL    string
	HTTPClient *http.Client

	// SubmitPath defaults to "/imagine" (ApiFrame / ImaginePro style).
	// GoAPI uses "/mj/v2/imagine" and TTAPI uses "/midjourney/imagine"
	// — pass the proxy's path here.
	SubmitPath string

	// FetchPath defaults to "/fetch/" — the trailing slash matters,
	// the task id is concatenated. Override for proxies that use a
	// different shape (e.g. "/task/" or "/mj/v2/fetch/").
	FetchPath string

	// AuthHeader controls how the API key is sent. Defaults to
	// "Authorization: Bearer <key>". Common alternatives: "X-API-Key",
	// "imagine-api-token", "Authorization: <key>" (no Bearer prefix).
	AuthHeader string
	AuthBearer bool // when true (default), prefix value with "Bearer "
}

func (c *ApiConfig) validate() error {
	if c == nil {
		return errors.New("midjourney: config must not be nil")
	}
	if c.ApiKey == nil {
		return errors.New("midjourney: ApiKey is required")
	}
	if c.BaseURL == "" {
		return errors.New("midjourney: BaseURL is required")
	}
	return nil
}

type Api struct {
	http       *resty.Client
	submitPath string
	fetchPath  string
}

func NewApi(cfg *ApiConfig) (*Api, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	client := resty.New().
		SetBaseURL(cfg.BaseURL).
		SetHeader("Content-Type", "application/json")

	header := cfg.AuthHeader
	if header == "" {
		header = "Authorization"
	}
	value := cfg.ApiKey.Get()
	if header == "Authorization" && cfg.AuthBearer {
		value = "Bearer " + value
	} else if header == "Authorization" && !cfg.AuthBearer && cfg.AuthHeader == "" {
		// Default to Bearer when caller didn't override Authorization.
		value = "Bearer " + value
	}
	client.SetHeader(header, value)

	if cfg.HTTPClient != nil {
		client.SetTransport(cfg.HTTPClient.Transport)
	}

	submit := cfg.SubmitPath
	if submit == "" {
		submit = "/imagine"
	}
	fetch := cfg.FetchPath
	if fetch == "" {
		fetch = "/fetch/"
	}

	return &Api{http: client, submitPath: submit, fetchPath: fetch}, nil
}

// GenerateRequest mirrors the common-denominator imagine payload.
// Different proxies accept overlapping but not identical fields; the
// untyped Extra map carries proxy-specific knobs through unchanged.
type GenerateRequest struct {
	Prompt      string         `json:"prompt"`
	AspectRatio string         `json:"aspect_ratio,omitempty"`
	Mode        string         `json:"mode,omitempty"` // "fast" / "relax" / "turbo"
	Webhook     string         `json:"webhook_endpoint,omitempty"`
	Extra       map[string]any `json:"-"`
}

// SubmitResponse is the proxy's enqueue reply. Different proxies use
// different field names for the task id (task_id / job_id / id / hash);
// the typed field captures the most common, and TaskIDFallback gives
// callers a way to extract from the raw response when the proxy uses
// something else.
type SubmitResponse struct {
	TaskID string         `json:"task_id"`
	JobID  string         `json:"job_id"`
	Hash   string         `json:"hash"`
	ID     string         `json:"id"`
	Raw    map[string]any `json:"-"`
}

func (s *SubmitResponse) ResolvedID() string {
	for _, v := range []string{s.TaskID, s.JobID, s.Hash, s.ID} {
		if v != "" {
			return v
		}
	}
	return ""
}

// FetchResponse is the poll body. Status moves through "PENDING" /
// "PROCESSING" / "SUCCESS" / "FAILURE" depending on the proxy — we
// treat anything with "SUCCESS"/"completed"/"done" as terminal-success
// and anything with "FAIL"/"error" as terminal-error.
type FetchResponse struct {
	Status     string   `json:"status"`
	Progress   string   `json:"progress"`
	ImageURL   string   `json:"image_url"`
	ImageURLs  []string `json:"image_urls"`
	URI        string   `json:"uri"`
	Result     string   `json:"result"`
	FailReason string   `json:"fail_reason"`
}

// Submit posts the prompt to the configured submit path.
func (a *Api) Submit(ctx context.Context, req *GenerateRequest) (*SubmitResponse, error) {
	if req == nil {
		return nil, errors.New("midjourney: request must not be nil")
	}
	var out SubmitResponse
	resp, err := a.http.R().SetContext(ctx).SetBody(req).SetResult(&out).Post(a.submitPath)
	if err != nil {
		return nil, fmt.Errorf("midjourney: submit failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("midjourney: http %d: %s", resp.StatusCode(), resp.String())
	}
	return &out, nil
}

// Fetch polls the status / result of a submitted task.
func (a *Api) Fetch(ctx context.Context, id string) (*FetchResponse, error) {
	var out FetchResponse
	resp, err := a.http.R().SetContext(ctx).SetResult(&out).Get(a.fetchPath + id)
	if err != nil {
		return nil, fmt.Errorf("midjourney: fetch failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("midjourney: http %d: %s", resp.StatusCode(), resp.String())
	}
	return &out, nil
}
