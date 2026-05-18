package evaluation

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/core/model/chat"
)

// ErrNilRequest is returned by every evaluator when the request
// pointer is nil. Callers can match with [errors.Is] to distinguish
// caller-side input errors from chat-model failures.
var ErrNilRequest = errors.New("evaluation: request must not be nil")

// llmEvaluator is the scored LLM-driven evaluator shared by
// [RelevancyEvaluator] and [FactCheckingEvaluator]. Concrete evaluators
// wrap it with the right default template plus a per-request variable
// binder; the call/response/scoring pipeline is identical.
//
// The prompt asks the model for a continuous score in [0, 1]; the
// resulting [Response.Score] is the parsed value and [Response.Pass]
// is derived as score >= threshold.
type llmEvaluator struct {
	chatClient     *chat.Client
	promptTemplate *chat.PromptTemplate
	bindVariables  func(*Request) map[string]any
	threshold      float64
}

// newLLMEvaluator builds a base evaluator. model + template + bind are
// required; threshold defaults to [DefaultPassThreshold] when 0.
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

// Evaluate renders the prompt with bindVariables(req), runs it through
// the chat client, and parses the scored answer into a [*Response].
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
