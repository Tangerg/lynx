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

type apiConfig struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

func (a apiConfig) validate() error {
	if a.APIKey == "" {
		return errors.New("stability: APIKey is required")
	}
	return nil
}

type api struct {
	http *resty.Client
}

func newAPI(config apiConfig) (*api, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}

	client := resty.New()
	if config.HTTPClient != nil {
		client = resty.NewWithClient(config.HTTPClient)
	}
	client.SetBaseURL(cmp.Or(config.BaseURL, DefaultBaseURL)).
		SetAuthToken(config.APIKey)

	return &api{http: client}, nil
}

// GenerateRequest models the union of fields each v2beta image endpoint
// accepts. Mode selects the response wrapping: [ResponseModeImage] returns
// raw bytes; [ResponseModeJSON] returns a base64 envelope with FinishReason
// + Seed echoed back (required when callers care about those).
type generateRequest struct {
	Prompt         string
	NegativePrompt string
	AspectRatio    string
	Model          string
	OutputFormat   string
	Seed           *int64
	StylePreset    string
	CFGScale       *float64
	Mode           string
}

type jsonResponse struct {
	Image        string `json:"image"`
	FinishReason string `json:"finish_reason"`
	Seed         int64  `json:"seed"`
}

func (a *api) generate(ctx context.Context, path string, req *generateRequest) ([]byte, http.Header, error) {
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

func buildFormFields(req *generateRequest) map[string]string {
	out := make(map[string]string)
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
	if req.CFGScale != nil {
		out["cfg_scale"] = strconv.FormatFloat(*req.CFGScale, 'f', -1, 64)
	}
	if req.Seed != nil {
		out["seed"] = strconv.FormatInt(*req.Seed, 10)
	}
	return out
}

func DecodeJSON(body []byte) (*jsonResponse, error) {
	var resp jsonResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("stability: decode json: %w", err)
	}
	return &resp, nil
}
