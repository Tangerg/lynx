package replicate

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

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
		return errors.New("replicate: config must not be nil")
	}
	if c.APIKey == nil {
		return errors.New("replicate: APIKey is required")
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

// PredictionRequest is the JSON body for both prediction endpoints.
// Input is a model-specific dictionary that Replicate forwards
// verbatim to the upstream model's input schema. Version is only set
// when posting to the community endpoint /v1/predictions.
type PredictionRequest struct {
	Input               map[string]any `json:"input"`
	Version             string         `json:"version,omitempty"`
	Webhook             string         `json:"webhook,omitempty"`
	WebhookEventsFilter []string       `json:"webhook_events_filter,omitzero"`
	Stream              bool           `json:"stream,omitzero"`
}

// PredictionResponse mirrors Replicate's prediction-job document.
// Status moves through "starting" → "processing" → "succeeded" /
// "failed" / "canceled". Output is model-specific: image models
// usually return []string (URLs) or a single string (URL); text models
// return []string (token chunks) or string.
type PredictionResponse struct {
	ID          string         `json:"id"`
	Model       string         `json:"model,omitempty"`
	Version     string         `json:"version,omitempty"`
	Status      string         `json:"status"`
	Input       map[string]any `json:"input,omitzero"`
	Output      any            `json:"output,omitempty"`
	Error       string         `json:"error,omitempty"`
	Logs        string         `json:"logs,omitempty"`
	CreatedAt   string         `json:"created_at,omitempty"`
	StartedAt   string         `json:"started_at,omitempty"`
	CompletedAt string         `json:"completed_at,omitempty"`
	URLs        struct {
		Get    string `json:"get"`
		Cancel string `json:"cancel"`
		Stream string `json:"stream,omitempty"`
	} `json:"urls"`
	Metrics struct {
		PredictTime float64 `json:"predict_time,omitempty"`
		TotalTime   float64 `json:"total_time,omitempty"`
	} `json:"metrics"`
}

// CreatePrediction submits a generation job. model accepts either
// "owner/name" (official model — routes to /v1/models/.../predictions)
// or "owner/name:version_hash" (community model — routes to
// /v1/predictions with version in body). The hash form lets callers
// pin to a specific community-uploaded snapshot.
func (a *API) CreatePrediction(ctx context.Context, modelID string, req *PredictionRequest) (*PredictionResponse, error) {
	if req == nil {
		return nil, errors.New("replicate: request must not be nil")
	}
	if modelID == "" {
		return nil, errors.New("replicate: model id must not be empty")
	}

	owner, name, version := parseModelID(modelID)
	if owner == "" || name == "" {
		return nil, fmt.Errorf("replicate: invalid model id %q (want owner/name[:version])", modelID)
	}

	var (
		path string
		body = *req
	)
	if version != "" {
		path = "/predictions"
		body.Version = version
	} else {
		path = fmt.Sprintf("/models/%s/%s/predictions", owner, name)
	}

	var out PredictionResponse
	resp, err := a.http.R().SetContext(ctx).SetBody(&body).SetResult(&out).Post(path)
	if err != nil {
		return nil, fmt.Errorf("replicate: submit failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("replicate: http %d: %s", resp.StatusCode(), resp.String())
	}
	return &out, nil
}

// GetPrediction polls a prediction's current state.
func (a *API) GetPrediction(ctx context.Context, id string) (*PredictionResponse, error) {
	if id == "" {
		return nil, errors.New("replicate: prediction id must not be empty")
	}
	var out PredictionResponse
	resp, err := a.http.R().SetContext(ctx).SetResult(&out).Get("/predictions/" + id)
	if err != nil {
		return nil, fmt.Errorf("replicate: poll failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("replicate: http %d: %s", resp.StatusCode(), resp.String())
	}
	return &out, nil
}

// CancelPrediction aborts a still-running prediction.
func (a *API) CancelPrediction(ctx context.Context, id string) (*PredictionResponse, error) {
	if id == "" {
		return nil, errors.New("replicate: prediction id must not be empty")
	}
	var out PredictionResponse
	resp, err := a.http.R().SetContext(ctx).SetResult(&out).Post("/predictions/" + id + "/cancel")
	if err != nil {
		return nil, fmt.Errorf("replicate: cancel failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("replicate: http %d: %s", resp.StatusCode(), resp.String())
	}
	return &out, nil
}

// parseModelID splits "owner/name[:version]" into its parts. Empty
// returns indicate the input wasn't well-formed.
func parseModelID(id string) (owner, name, version string) {
	owner, rest, ok := strings.Cut(id, "/")
	if !ok {
		return "", "", ""
	}
	if n, v, hasVersion := strings.Cut(rest, ":"); hasVersion {
		return owner, n, v
	}
	return owner, rest, ""
}
