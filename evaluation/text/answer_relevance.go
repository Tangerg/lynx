package text

import (
	"context"

	"github.com/Tangerg/scope/evaluation"
)

const MetricAnswerRelevance evaluation.MetricName = "answer_relevance"

const answerRelevancePrompt = `Evaluate how directly and completely the output addresses the input.

Score relevance from 0.0 to 1.0 and provide concise feedback.

Input:
{{.Input}}

Output:
{{.Output}}

Evaluation:`

// AnswerRelevanceEvaluator scores whether generated output addresses its
// originating input. Groundedness is intentionally evaluated separately.
type AnswerRelevanceEvaluator struct {
	evaluator evaluation.Evaluator[AnswerRelevanceSample]
}

type answerRelevanceVariables struct {
	Input  string
	Output string
}

func NewAnswerRelevanceEvaluator(config ModelEvaluatorConfig) (*AnswerRelevanceEvaluator, error) {
	metric, err := evaluation.NewMetric(evaluation.MetricConfig{Namespace: "text", Name: MetricAnswerRelevance})
	if err != nil {
		return nil, err
	}
	evaluator, err := newModelEvaluator(
		config,
		metric,
		answerRelevancePrompt,
		func(sample AnswerRelevanceSample) answerRelevanceVariables {
			return answerRelevanceVariables(sample)
		},
		templateInputName,
		templateOutputName,
	)
	if err != nil {
		return nil, err
	}
	return &AnswerRelevanceEvaluator{evaluator: evaluator}, nil
}

func (evaluator *AnswerRelevanceEvaluator) Evaluate(ctx context.Context, sample AnswerRelevanceSample) (evaluation.Report, error) {
	if err := sample.Validate(); err != nil {
		return evaluation.Report{}, err
	}
	return evaluator.evaluator.Evaluate(ctx, sample)
}

var _ evaluation.Evaluator[AnswerRelevanceSample] = (*AnswerRelevanceEvaluator)(nil)
