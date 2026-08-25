package evaluation

import (
	"context"
	"fmt"
	"strings"
)

const factPrompt = `Evaluate how well the answer is supported by the provided context.

Score support from 0.0 to 1.0 and provide concise feedback.

Context:
{{.Context}}

Answer:
{{.Answer}}

Evaluation:`

// FactEvaluator scores whether an answer is supported by source context.
type FactEvaluator struct {
	evaluator *modelEvaluator
}

// NewFactEvaluator constructs a fact-support evaluator.
func NewFactEvaluator(config ModelConfig) (*FactEvaluator, error) {
	evaluator, err := newModelEvaluator(config, factPrompt, validateFactRequest, "Answer", "Context")
	if err != nil {
		return nil, err
	}
	return &FactEvaluator{evaluator: evaluator}, nil
}

// Evaluate scores request for factual support.
func (e *FactEvaluator) Evaluate(ctx context.Context, request Request) (Result, error) {
	return e.evaluator.Evaluate(ctx, request)
}

func validateFactRequest(request Request) error {
	if strings.TrimSpace(request.Answer) == "" {
		return fmt.Errorf("%w: answer is required", ErrInvalidRequest)
	}
	if request.contextText() == "" {
		return fmt.Errorf("%w: context is required", ErrInvalidRequest)
	}
	return nil
}
