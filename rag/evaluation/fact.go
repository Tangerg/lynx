package evaluation

import (
	"fmt"
	"strings"
)

const factPrompt = `Evaluate how well the answer is supported by the provided context.

Reply with a number between 0.0 and 1.0 on the first line, then briefly explain your reasoning.

Context:
{{.Context}}

Answer:
{{.Answer}}

Score:`

// FactEvaluator scores whether an answer is supported by source context.
type FactEvaluator struct {
	*modelEvaluator
}

// NewFactEvaluator constructs a fact-support evaluator.
func NewFactEvaluator(config ModelConfig) (*FactEvaluator, error) {
	evaluator, err := newModelEvaluator(config, factPrompt, validateFactRequest)
	if err != nil {
		return nil, err
	}
	return &FactEvaluator{modelEvaluator: evaluator}, nil
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
