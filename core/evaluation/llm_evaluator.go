package evaluation

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/core/model/chat"
)

var ErrNilRequest = errors.New("evaluation: request must not be nil")

type llmEvaluator struct {
	chatClient     *chat.Client
	promptTemplate *chat.PromptTemplate
	bindVariables  func(*Request) map[string]any
	threshold      float64
}

func newLLMEvaluator(
	model chat.Model,
	template *chat.PromptTemplate,
	bind func(*Request) map[string]any,
	threshold float64,
) (*llmEvaluator, error) {
	client, err := chat.NewClient(model)
	if err != nil {
		return nil, err
	}
	if threshold <= 0 {
		threshold = DefaultPassThreshold
	}
	return &llmEvaluator{
		chatClient:     client,
		promptTemplate: template,
		bindVariables:  bind,
		threshold:      threshold,
	}, nil
}

func (e *llmEvaluator) Evaluate(ctx context.Context, req *Request) (*Response, error) {
	if req == nil {
		return nil, ErrNilRequest
	}
	rendered := e.promptTemplate.Clone()
	for key, value := range e.bindVariables(req) {
		rendered = rendered.WithVariable(key, value)
	}
	text, _, err := e.chatClient.
		ChatWithPromptTemplate(rendered).
		Call().
		Text(ctx)
	if err != nil {
		return nil, fmt.Errorf("evaluation.llmEvaluator.Evaluate: chat call: %w", err)
	}
	return parseScoredResponse(text, e.threshold)
}
