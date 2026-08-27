package evaluation

import (
	"context"
	"fmt"
	"strings"
)

const groundednessPrompt = `Evaluate how well the output is supported by the provided context.

Score support from 0.0 to 1.0 and provide concise feedback.

Context:
{{.Context}}

Output:
{{.Output}}

Evaluation:`

// GroundednessEvaluator scores whether generated output is supported by the
// supplied evidence.
type GroundednessEvaluator struct {
	evaluator *modelEvaluator
}

// NewGroundednessEvaluator constructs a groundedness evaluator.
func NewGroundednessEvaluator(config ModelConfig) (*GroundednessEvaluator, error) {
	evaluator, err := newModelEvaluator(
		config,
		MetricGroundedness,
		groundednessPrompt,
		validateGroundednessSample,
		"Output",
		"Context",
	)
	if err != nil {
		return nil, err
	}
	return &GroundednessEvaluator{evaluator: evaluator}, nil
}

// Evaluate scores sample for factual support.
func (g *GroundednessEvaluator) Evaluate(ctx context.Context, sample TextSample) (Report, error) {
	return g.evaluator.Evaluate(ctx, sample)
}

func validateGroundednessSample(sample TextSample) error {
	if strings.TrimSpace(sample.Output) == "" {
		return fmt.Errorf("%w: output is required", ErrInvalidSample)
	}
	if sample.ContextText() == "" {
		return fmt.Errorf("%w: context is required", ErrInvalidSample)
	}
	return nil
}
