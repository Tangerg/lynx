package text

import (
	"context"

	"github.com/Tangerg/scope/evaluation"
)

const MetricAnswerRelevance evaluation.Metric = "answer_relevance"

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
	evaluator *modelEvaluator
}

func NewAnswerRelevanceEvaluator(config ModelEvaluatorConfig) (*AnswerRelevanceEvaluator, error) {
	evaluator, err := newModelEvaluator(
		config,
		MetricAnswerRelevance,
		answerRelevancePrompt,
		templateInputName,
		templateOutputName,
	)
	if err != nil {
		return nil, err
	}
	return &AnswerRelevanceEvaluator{evaluator: evaluator}, nil
}

func (evaluator *AnswerRelevanceEvaluator) Evaluate(ctx context.Context, sample Sample) (evaluation.Report, error) {
	if err := sample.validateAnswerRelevance(); err != nil {
		return evaluation.Report{}, err
	}
	return evaluator.evaluator.evaluate(ctx, sample)
}

var _ evaluation.Evaluator[Sample] = (*AnswerRelevanceEvaluator)(nil)
