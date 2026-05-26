package jina

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
		return errors.New("jina: config must not be nil")
	}
	if c.APIKey == nil {
		return errors.New("jina: APIKey is required")
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

	var client *resty.Client
	if cfg.HTTPClient != nil {
		client = resty.NewWithClient(cfg.HTTPClient)
	} else {
		client = resty.New()
	}
	client.
		SetBaseURL(cmp.Or(cfg.BaseURL, DefaultBaseURL)).
		SetAuthToken(cfg.APIKey.Get()).
		SetHeader("Content-Type", "application/json").
		SetHeader("Accept", "application/json")

	return &API{http: client}, nil
}

type EmbeddingRequest struct {
	Model           string   `json:"model"`
	Input           []string `json:"input"`
	Task            string   `json:"task,omitempty"`
	LateChunking    *bool    `json:"late_chunking,omitempty"`
	Dimensions      *int64   `json:"dimensions,omitempty"`
	Truncate        *bool    `json:"truncate,omitempty"`
	EmbeddingType   string   `json:"embedding_type,omitempty"`
	Normalized      *bool    `json:"normalized,omitempty"`
}

type EmbeddingResponse struct {
	Object string `json:"object"`
	Model  string `json:"model"`
	Data   []struct {
		Object    string    `json:"object"`
		Index     int64     `json:"index"`
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
	Usage struct {
		TotalTokens  int64 `json:"total_tokens"`
		PromptTokens int64 `json:"prompt_tokens"`
	} `json:"usage"`
}

func (a *API) Embedding(ctx context.Context, req *EmbeddingRequest) (*EmbeddingResponse, error) {
	if req == nil {
		return nil, errors.New("jina: request must not be nil")
	}
	if len(req.Input) == 0 {
		return nil, errors.New("jina: input must not be empty")
	}

	var out EmbeddingResponse
	resp, err := a.http.R().
		SetContext(ctx).
		SetBody(req).
		SetResult(&out).
		Post("/embeddings")
	if err != nil {
		return nil, fmt.Errorf("jina: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("jina: http %d: %s", resp.StatusCode(), resp.String())
	}
	return &out, nil
}
