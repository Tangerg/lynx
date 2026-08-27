package evaluation

import (
	"context"
	"fmt"
	"strings"
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

// NewAnswerRelevanceEvaluator constructs an answer-relevance evaluator.
func NewAnswerRelevanceEvaluator(config ModelConfig) (*AnswerRelevanceEvaluator, error) {
	evaluator, err := newModelEvaluator(
		config,
		MetricAnswerRelevance,
		answerRelevancePrompt,
		validateAnswerRelevanceSample,
		"Input",
		"Output",
	)
	if err != nil {
		return nil, err
	}
	return &AnswerRelevanceEvaluator{evaluator: evaluator}, nil
}

// Evaluate scores sample for answer relevance.
func (a *AnswerRelevanceEvaluator) Evaluate(ctx context.Context, sample TextSample) (Report, error) {
	return a.evaluator.Evaluate(ctx, sample)
}

func validateAnswerRelevanceSample(sample TextSample) error {
	if strings.TrimSpace(sample.Input) == "" {
		return fmt.Errorf("%w: input is required", ErrInvalidSample)
	}
	if strings.TrimSpace(sample.Output) == "" {
		return fmt.Errorf("%w: output is required", ErrInvalidSample)
	}
	return nil
}
