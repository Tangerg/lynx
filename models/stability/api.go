package stability

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-resty/resty/v2"
)

type APIConfig struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

func (c APIConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("stability: APIKey is required")
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
		SetAuthToken(cfg.APIKey)
	if cfg.HTTPClient != nil {
		client.SetTransport(cfg.HTTPClient.Transport)
	}

	return &API{http: client}, nil
}

// GenerateRequest models the union of fields each v2beta image endpoint
// accepts. Mode selects the response wrapping: [ResponseModeImage] returns
// raw bytes; [ResponseModeJSON] returns a base64 envelope with FinishReason
// + Seed echoed back (required when callers care about those).
type GenerateRequest struct {
	Prompt         string
	NegativePrompt string
	AspectRatio    string
	Model          string
	OutputFormat   string
	Seed           *int64
	StylePreset    string
	Mode           string
}

type JSONResponse struct {
	Image        string `json:"image"`
	FinishReason string `json:"finish_reason"`
	Seed         int64  `json:"seed"`
}

func (a *API) Generate(ctx context.Context, path string, req *GenerateRequest) ([]byte, http.Header, error) {
	if req == nil {
		return nil, nil, errors.New("stability: request must not be nil")
	}

	r := a.http.R().
		SetContext(ctx).
		SetMultipartFormData(buildFormFields(req)).
		SetHeader("Accept", cmp.Or(req.Mode, ResponseModeImage))

	resp, err := r.Post(path)
	if err != nil {
		return nil, nil, fmt.Errorf("stability: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, nil, fmt.Errorf("stability: http %d: %s", resp.StatusCode(), resp.String())
	}
	return resp.Body(), resp.Header(), nil
}

func buildFormFields(req *GenerateRequest) map[string]string {
	out := make(map[string]string, 8)
	put := func(k, v string) {
		if v != "" {
			out[k] = v
		}
	}
	put("prompt", req.Prompt)
	put("negative_prompt", req.NegativePrompt)
	put("aspect_ratio", req.AspectRatio)
	put("model", req.Model)
	put("output_format", req.OutputFormat)
	put("style_preset", req.StylePreset)
	if req.Seed != nil {
		out["seed"] = strconv.FormatInt(*req.Seed, 10)
	}
	return out
}

func DecodeJSON(body []byte) (*JSONResponse, error) {
	var resp JSONResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("stability: decode json: %w", err)
	}
	return &resp, nil
}
