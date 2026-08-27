package evaluation

import (
	"context"
	"fmt"
	"strings"

	"github.com/samber/lo"
)

// CompositeEvaluator evaluates children sequentially, applies AND semantics
// to Passed, and averages normalized scores. It is immutable after construction.
type CompositeEvaluator[T any] struct {
	evaluators []Evaluator[T]
}

type reportCollection []Report

func (reports reportCollection) combine() (Report, error) {
	if len(reports) == 0 {
		return Report{}, fmt.Errorf("%w: no child reports to combine", ErrInvalidReport)
	}
	if len(reports) == 1 {
		return reports[0].Clone(), nil
	}

	combined := Report{Metric: MetricComposite, Passed: true, Details: reports}
	feedback := make([]string, 0, len(reports))
	for _, report := range reports {
		combined.Passed = combined.Passed && report.Passed
		combined.Score += report.Score
		if report.Feedback != "" {
			feedback = append(feedback, report.Feedback)
		}
	}
	combined.Score /= Score(len(reports))
	combined.Feedback = strings.Join(feedback, "\n\n")
	return combined, nil
}

// NewCompositeEvaluator snapshots evaluators. At least one non-nil evaluator is
// required.
func NewCompositeEvaluator[T any](evaluators ...Evaluator[T]) (*CompositeEvaluator[T], error) {
	if len(evaluators) == 0 {
		return nil, fmt.Errorf("%w: at least one evaluator is required", ErrInvalidEvaluatorConfig)
	}
	snapshot := make([]Evaluator[T], len(evaluators))
	for index, evaluator := range evaluators {
		if lo.IsNil(evaluator) {
			return nil, fmt.Errorf("%w: evaluators[%d] is nil", ErrInvalidEvaluatorConfig, index)
		}
		snapshot[index] = evaluator
	}
	return &CompositeEvaluator[T]{evaluators: snapshot}, nil
}

// Evaluate stops on the first child error or invalid child report. Error
// wrapping preserves errors.Is identities.
func (composite *CompositeEvaluator[T]) Evaluate(ctx context.Context, subject T) (Report, error) {
	reports := make(reportCollection, 0, len(composite.evaluators))
	for index, evaluator := range composite.evaluators {
		if err := ctx.Err(); err != nil {
			return Report{}, err
		}
		report, err := evaluator.Evaluate(ctx, subject)
		if err != nil {
			return Report{}, fmt.Errorf("evaluation: evaluator %d: %w", index, err)
		}
		if err := report.Validate(); err != nil {
			return Report{}, fmt.Errorf("evaluation: evaluator %d: %w", index, err)
		}
		reports = append(reports, report.Clone())
	}
	return reports.combine()
}
