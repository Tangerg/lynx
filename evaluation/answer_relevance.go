package evaluation

import (
	"context"
)

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
		"Input",
		"Output",
	)
	if err != nil {
		return nil, err
	}
	return &AnswerRelevanceEvaluator{evaluator: evaluator}, nil
}

func (evaluator *AnswerRelevanceEvaluator) Evaluate(ctx context.Context, sample TextSample) (Report, error) {
	if err := sample.validateAnswerRelevance(); err != nil {
		return Report{}, err
	}
	return evaluator.evaluator.evaluate(ctx, sample)
}
