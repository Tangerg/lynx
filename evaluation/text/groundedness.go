package text

import (
	"context"

	"github.com/Tangerg/scope/evaluation"
)

const MetricGroundedness evaluation.Metric = "groundedness"

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
		templateOutputName,
		templateContextName,
	)
	if err != nil {
		return nil, err
	}
	return &GroundednessEvaluator{evaluator: evaluator}, nil
}

func (evaluator *GroundednessEvaluator) Evaluate(ctx context.Context, sample Sample) (evaluation.Report, error) {
	if err := sample.validateGroundedness(); err != nil {
		return evaluation.Report{}, err
	}
	return evaluator.evaluator.evaluate(ctx, sample)
}

var _ evaluation.Evaluator[Sample] = (*GroundednessEvaluator)(nil)
