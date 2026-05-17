package google

import (
	"context"
	"errors"

	"google.golang.org/genai"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/tokenizer"
)

// TextEstimatorConfig configures a Gemini-backed token estimator.
// Token counts vary across model families — supply the same Model name
// you intend to send chat requests under so the count matches the real
// billing.
type TextEstimatorConfig struct {
	ApiKey   model.ApiKey
	Model    string
	Backend  genai.Backend
	Project  string
	Location string
}

func (c *TextEstimatorConfig) validate() error {
	if c == nil {
		return errors.New("google: config must not be nil")
	}
	if c.Backend != genai.BackendVertexAI && c.ApiKey == nil {
		return errors.New("google: ApiKey is required")
	}
	if c.Model == "" {
		return errors.New("google: DefaultOptions is required")
	}
	return nil
}

var _ tokenizer.TextEstimator = (*TextEstimator)(nil)

// TextEstimator reports input-token counts via Gemini's count_tokens
// endpoint. Implements [tokenizer.TextEstimator] so it drops into code
// paths gating on token budgets (RAG chunking, prompt-window checks).
type TextEstimator struct {
	api   *Api
	model string
}

func NewTextEstimator(cfg *TextEstimatorConfig) (*TextEstimator, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	api, err := NewApi(&ApiConfig{
		ApiKey:   cfg.ApiKey,
		Backend:  cfg.Backend,
		Project:  cfg.Project,
		Location: cfg.Location,
	})
	if err != nil {
		return nil, err
	}

	return &TextEstimator{api: api, model: cfg.Model}, nil
}

// EstimateText returns the prompt-token count Gemini would charge if
// text were sent as a single user message under the configured model.
func (t *TextEstimator) EstimateText(ctx context.Context, text string) (int, error) {
	contents := []*genai.Content{genai.NewContentFromText(text, genai.RoleUser)}
	resp, err := t.api.CountTokens(ctx, t.model, contents, nil)
	if err != nil {
		return 0, err
	}
	return int(resp.TotalTokens), nil
}
