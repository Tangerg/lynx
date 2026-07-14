package prodia

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-resty/resty/v2"
)

type APIConfig struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

func (c APIConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("prodia: APIKey is required")
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
		SetHeader("Authorization", "Bearer "+cfg.APIKey)
	if cfg.HTTPClient != nil {
		client.SetTransport(cfg.HTTPClient.Transport)
	}
	return &API{http: client}, nil
}

// JobRequest mirrors POST /job. Prodia routes via a "type" discriminator
// — typical values: "inference.flux.dev.txt2img.v1", "inference.flux.schnell.txt2img.v1",
// "inference.sd1.5.txt2img.v1", "inference.sdxl.txt2img.v1".
type JobRequest struct {
	Type   string         `json:"type"`
	Config map[string]any `json:"config"`
}

// Job submits a job and returns the raw image bytes when status is 200.
// Prodia's v2 endpoint is sync — it blocks until the image is ready
// (typically 1-5s) and returns the binary directly with content-type
// image/jpeg or image/png.
func (a *API) Job(ctx context.Context, req *JobRequest, accept string) ([]byte, http.Header, error) {
	if req == nil {
		return nil, nil, errors.New("prodia: request must not be nil")
	}
	resp, err := a.http.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetHeader("Accept", cmp.Or(accept, "image/*")).
		SetBody(req).
		Post("/job")
	if err != nil {
		return nil, nil, fmt.Errorf("prodia: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, nil, fmt.Errorf("prodia: http %d: %s", resp.StatusCode(), resp.String())
	}
	return resp.Body(), resp.Header(), nil
}
