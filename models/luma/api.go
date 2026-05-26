package luma

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
		return errors.New("luma: config must not be nil")
	}
	if c.APIKey == nil {
		return errors.New("luma: APIKey is required")
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
		SetAuthToken(cfg.APIKey.Get()).
		SetHeader("Content-Type", "application/json")
	if cfg.HTTPClient != nil {
		client.SetTransport(cfg.HTTPClient.Transport)
	}
	return &API{http: client}, nil
}

// ImageGenerateRequest mirrors POST /generations/image. Luma uses
// Photon for stills (Dream Machine handles video — out of lynx scope).
type ImageGenerateRequest struct {
	Prompt      string `json:"prompt"`
	AspectRatio string `json:"aspect_ratio,omitempty"`
	Model       string `json:"model,omitempty"`
	// ImageRef is an array of {url, weight}; we keep it as map[string]any
	// so callers can thread richer refs via Extra without us prescribing
	// the exact wire shape.
	ImageRef       []map[string]any `json:"image_ref,omitzero"`
	StyleRef       []map[string]any `json:"style_ref,omitzero"`
	CharacterRef   map[string]any   `json:"character_ref,omitzero"`
	ModifyImageRef map[string]any   `json:"modify_image_ref,omitzero"`
}

// Generation is the poll-result body. State moves through "queued" /
// "dreaming" / "completed" / "failed".
type Generation struct {
	ID            string `json:"id"`
	State         string `json:"state"`
	FailureReason string `json:"failure_reason"`
	Assets        struct {
		Image string `json:"image"`
		Video string `json:"video"`
	} `json:"assets"`
}

func (a *API) GenerateImage(ctx context.Context, req *ImageGenerateRequest) (*Generation, error) {
	if req == nil {
		return nil, errors.New("luma: request must not be nil")
	}
	var out Generation
	resp, err := a.http.R().SetContext(ctx).SetBody(req).SetResult(&out).Post("/generations/image")
	if err != nil {
		return nil, fmt.Errorf("luma: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("luma: http %d: %s", resp.StatusCode(), resp.String())
	}
	return &out, nil
}

func (a *API) GetGeneration(ctx context.Context, id string) (*Generation, error) {
	var out Generation
	resp, err := a.http.R().SetContext(ctx).SetResult(&out).Get("/generations/" + id)
	if err != nil {
		return nil, fmt.Errorf("luma: poll failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("luma: http %d: %s", resp.StatusCode(), resp.String())
	}
	return &out, nil
}
