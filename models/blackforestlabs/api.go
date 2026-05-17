package blackforestlabs

import (
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
		return errors.New("blackforestlabs: config must not be nil")
	}
	if c.ApiKey == nil {
		return errors.New("blackforestlabs: ApiKey is required")
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
		SetHeader("x-key", cfg.ApiKey.Get()).
		SetHeader("Content-Type", "application/json")
	if cfg.HTTPClient != nil {
		client.SetTransport(cfg.HTTPClient.Transport)
	}
	return &Api{http: client}, nil
}

// GenerateRequest is the union of fields the various Flux endpoints
// accept. Each Flux model (flux-pro-1.1, flux-pro-1.1-ultra,
// flux-kontext-pro, ...) has its own endpoint path; we forward
// everything and let the API reject unknown fields.
type GenerateRequest struct {
	Prompt           string `json:"prompt"`
	ImagePrompt      string `json:"image_prompt,omitempty"`
	Width            int    `json:"width,omitempty"`
	Height           int    `json:"height,omitempty"`
	AspectRatio      string `json:"aspect_ratio,omitempty"`
	PromptUpsampling bool   `json:"prompt_upsampling,omitzero"`
	Seed             *int64 `json:"seed,omitempty"`
	SafetyTolerance  *int   `json:"safety_tolerance,omitempty"`
	OutputFormat     string `json:"output_format,omitempty"`
	Raw              bool   `json:"raw,omitzero"`
}

// AsyncResponse is the body of any POST /v1/<model> call — it returns a
// task id which the caller polls via GetResult.
type AsyncResponse struct {
	ID         string `json:"id"`
	PollingURL string `json:"polling_url"`
}

// PollResult is the body of GET /v1/get_result?id=... — Status moves
// through "Pending" / "Ready" / "Error" / "Content Moderated".
type PollResult struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Result struct {
		Sample   string `json:"sample"`
		Prompt   string `json:"prompt"`
		Seed     int64  `json:"seed"`
		Duration int64  `json:"duration"`
	} `json:"result"`
}

// Generate posts to /<model> (e.g. "flux-pro-1.1", "flux-kontext-pro").
func (a *Api) Generate(ctx context.Context, model string, req *GenerateRequest) (*AsyncResponse, error) {
	if req == nil {
		return nil, errors.New("blackforestlabs: request must not be nil")
	}
	var out AsyncResponse
	resp, err := a.http.R().SetContext(ctx).SetBody(req).SetResult(&out).Post("/" + model)
	if err != nil {
		return nil, fmt.Errorf("blackforestlabs: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("blackforestlabs: http %d: %s", resp.StatusCode(), resp.String())
	}
	return &out, nil
}

// GetResult fetches the current task state. Status is what the caller
// polls on; Result.Sample is a signed URL to the generated PNG/JPEG
// (valid for ~10 minutes).
func (a *Api) GetResult(ctx context.Context, id string) (*PollResult, error) {
	var out PollResult
	resp, err := a.http.R().SetContext(ctx).SetQueryParam("id", id).SetResult(&out).Get("/get_result")
	if err != nil {
		return nil, fmt.Errorf("blackforestlabs: poll failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("blackforestlabs: http %d: %s", resp.StatusCode(), resp.String())
	}
	return &out, nil
}
