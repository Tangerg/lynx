package voyage

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
		return errors.New("voyage: config must not be nil")
	}
	if c.APIKey == nil {
		return errors.New("voyage: APIKey is required")
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

type EmbeddingRequest struct {
	Input           []string `json:"input"`
	Model           string   `json:"model"`
	InputType       string   `json:"input_type,omitempty"`
	Truncation      *bool    `json:"truncation,omitempty"`
	OutputDimension *int64   `json:"output_dimension,omitempty"`
	OutputDtype     string   `json:"output_dtype,omitempty"`
	EncodingFormat  string   `json:"encoding_format,omitempty"`
}

type EmbeddingResponse struct {
	Object string `json:"object"`
	Data   []struct {
		Object    string    `json:"object"`
		Embedding []float64 `json:"embedding"`
		Index     int64     `json:"index"`
	} `json:"data"`
	Model string `json:"model"`
	Usage struct {
		TotalTokens int64 `json:"total_tokens"`
	} `json:"usage"`
}

func (a *API) Embedding(ctx context.Context, req *EmbeddingRequest) (*EmbeddingResponse, error) {
	if req == nil {
		return nil, errors.New("voyage: request must not be nil")
	}

	var out EmbeddingResponse
	resp, err := a.http.R().
		SetContext(ctx).
		SetBody(req).
		SetResult(&out).
		Post("/embeddings")
	if err != nil {
		return nil, fmt.Errorf("voyage: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("voyage: http %d: %s", resp.StatusCode(), resp.String())
	}
	return &out, nil
}
