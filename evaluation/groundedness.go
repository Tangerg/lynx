package evaluation

import (
	"context"
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

func NewGroundednessEvaluator(config ModelEvaluatorConfig) (*GroundednessEvaluator, error) {
	evaluator, err := newModelEvaluator(
		config,
		MetricGroundedness,
		groundednessPrompt,
		"Output",
		"Context",
	)
	if err != nil {
		return nil, err
	}
	return &GroundednessEvaluator{evaluator: evaluator}, nil
}

func (evaluator *GroundednessEvaluator) Evaluate(ctx context.Context, sample TextSample) (Report, error) {
	if err := sample.validateGroundedness(); err != nil {
		return Report{}, err
	}
	return evaluator.evaluator.evaluate(ctx, sample)
}
