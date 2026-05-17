package nomic

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
		return errors.New("nomic: config must not be nil")
	}
	if c.ApiKey == nil {
		return errors.New("nomic: ApiKey is required")
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

	var client *resty.Client
	if cfg.HTTPClient != nil {
		client = resty.NewWithClient(cfg.HTTPClient)
	} else {
		client = resty.New()
	}
	client.
		SetBaseURL(cmp.Or(cfg.BaseURL, DefaultBaseURL)).
		SetAuthToken(cfg.ApiKey.Get()).
		SetHeader("Content-Type", "application/json").
		SetHeader("Accept", "application/json")

	return &Api{http: client}, nil
}

type EmbeddingRequest struct {
	Model          string   `json:"model"`
	Texts          []string `json:"texts"`
	TaskType       string   `json:"task_type,omitempty"`
	Dimensionality *int64   `json:"dimensionality,omitempty"`
	LongTextMode   string   `json:"long_text_mode,omitempty"`
}

type EmbeddingResponse struct {
	Embeddings [][]float64 `json:"embeddings"`
	Model      string      `json:"model"`
	Usage      struct {
		PromptTokens int64 `json:"prompt_tokens"`
		TotalTokens  int64 `json:"total_tokens"`
	} `json:"usage"`
}

func (a *Api) Embedding(ctx context.Context, req *EmbeddingRequest) (*EmbeddingResponse, error) {
	if req == nil {
		return nil, errors.New("nomic: request must not be nil")
	}
	if len(req.Texts) == 0 {
		return nil, errors.New("nomic: texts must not be empty")
	}

	var out EmbeddingResponse
	resp, err := a.http.R().
		SetContext(ctx).
		SetBody(req).
		SetResult(&out).
		Post("/embedding/text")
	if err != nil {
		return nil, fmt.Errorf("nomic: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("nomic: http %d: %s", resp.StatusCode(), resp.String())
	}
	return &out, nil
}
