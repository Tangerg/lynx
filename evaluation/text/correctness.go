package text

import (
	"context"

	"github.com/Tangerg/scope/evaluation"
)

const MetricCorrectness evaluation.MetricName = "correctness"

const correctnessPrompt = `Evaluate whether the output is correct for the input using the reference answer as evidence.

Score correctness from 0.0 to 1.0 and provide concise feedback.

Input:
{{.Input}}

Reference:
{{.Reference}}

Output:
{{.Output}}

Evaluation:`

type CorrectnessEvaluator struct {
	evaluator evaluation.Evaluator[CorrectnessSample]
}

type correctnessVariables struct {
	Input     string
	Output    string
	Reference string
}

func NewCorrectnessEvaluator(config ModelEvaluatorConfig) (*CorrectnessEvaluator, error) {
	metric, err := evaluation.NewMetric(evaluation.MetricConfig{Namespace: "text", Name: MetricCorrectness})
	if err != nil {
		return nil, err
	}
	evaluator, err := newModelEvaluator(
		config,
		metric,
		correctnessPrompt,
		func(sample CorrectnessSample) correctnessVariables {
			return correctnessVariables(sample)
		},
		templateInputName,
		templateOutputName,
		templateReferenceName,
	)
	if err != nil {
		return nil, err
	}
	return &CorrectnessEvaluator{evaluator: evaluator}, nil
}

func (evaluator *CorrectnessEvaluator) Evaluate(ctx context.Context, sample CorrectnessSample) (evaluation.Report, error) {
	if err := sample.Validate(); err != nil {
		return evaluation.Report{}, err
	}
	return evaluator.evaluator.Evaluate(ctx, sample)
}

var _ evaluation.Evaluator[CorrectnessSample] = (*CorrectnessEvaluator)(nil)
