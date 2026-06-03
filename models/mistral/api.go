package mistral

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-resty/resty/v2"

	"github.com/Tangerg/lynx/core/model"
)

// API covers the Mistral-specific endpoints that don't already exist in
// the OpenAI-compatible surface — moderation, OCR, etc. Chat and
// embeddings go through the openai provider with a swapped base URL
// (see [NewChatModel] / [NewEmbeddingModel] below).
type APIConfig struct {
	APIKey     model.APIKey
	BaseURL    string
	HTTPClient *http.Client
}

func (c APIConfig) Validate() error {
	if c.APIKey == nil {
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
	client := resty.New().
		SetBaseURL(cmp.Or(cfg.BaseURL, DefaultBaseURL)).
		SetAuthToken(cfg.APIKey.Get()).
		SetHeader("Content-Type", "application/json")
	if cfg.HTTPClient != nil {
		client.SetTransport(cfg.HTTPClient.Transport)
	}
	return &API{http: client}, nil
}

// ModerationRequest mirrors POST /moderations. Mistral's moderation API
// takes a free-form `input` (string or array of strings) plus a model
// id ("mistral-moderation-latest" is current).
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
