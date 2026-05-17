package anthropic

import (
	"context"
	"errors"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/tokenizer"
)

// TextEstimatorConfig configures an Anthropic-backed token estimator.
// Model picks the tokenizer vocabulary Anthropic counts against; a
// mismatch (Claude 3 model name vs Claude 4 vocab) produces wrong
// counts.
type TextEstimatorConfig struct {
	ApiKey         model.ApiKey
	Model          string
	RequestOptions []option.RequestOption
}

func (c *TextEstimatorConfig) validate() error {
	if c == nil {
		return errors.New("anthropic: config must not be nil")
	}
	if c.ApiKey == nil {
		return errors.New("anthropic: ApiKey is required")
	}
	if c.Model == "" {
		return errors.New("anthropic: DefaultOptions is required")
	}
	return nil
}

var _ tokenizer.TextEstimator = (*TextEstimator)(nil)

// TextEstimator reports input-token counts via Anthropic's
// /messages/count_tokens endpoint. Implements [tokenizer.TextEstimator]
// so it drops into code paths already gating on token budgets (RAG
// chunking, prompt-window checks, cost preflight).
//
// Every estimate is a network round-trip; for high-QPS counting reach
// for an offline tokenizer instead.
type TextEstimator struct {
	api   *Api
	model string
}

func NewTextEstimator(cfg *TextEstimatorConfig) (*TextEstimator, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	api, err := NewApi(&ApiConfig{
		ApiKey:         cfg.ApiKey,
		RequestOptions: cfg.RequestOptions,
	})
	if err != nil {
		return nil, err
	}

	return &TextEstimator{api: api, model: cfg.Model}, nil
}

// EstimateText returns the prompt-token count Anthropic would charge if
// text were sent as a single user message under the configured model.
func (t *TextEstimator) EstimateText(ctx context.Context, text string) (int, error) {
	resp, err := t.api.CountTokens(ctx, &anthropicsdk.MessageCountTokensParams{
		Model: anthropicsdk.Model(t.model),
		Messages: []anthropicsdk.MessageParam{
			anthropicsdk.NewUserMessage(anthropicsdk.NewTextBlock(text)),
		},
	})
	if err != nil {
		return 0, err
	}
	return int(resp.InputTokens), nil
}
