package evaluation

import (
	"context"
	"fmt"
	"strings"

	"github.com/samber/lo"
)

// Composite evaluates children sequentially, applies AND semantics to Passed,
// and averages normalized scores. It is immutable after construction.
type Composite[T any] struct {
	evaluators []Evaluator[T]
}

type reports []Report

func (r reports) aggregate() (Report, error) {
	if len(r) == 0 {
		return Report{}, fmt.Errorf("%w: no reports to merge", ErrInvalidReport)
	}
	if len(r) == 1 {
		return r[0].Clone(), nil
	}

	merged := Report{Metric: MetricComposite, Passed: true, Details: r}
	feedback := make([]string, 0, len(r))
	for _, report := range r {
		merged.Passed = merged.Passed && report.Passed
		merged.Score += report.Score
		if report.Feedback != "" {
			feedback = append(feedback, report.Feedback)
		}
	}
	merged.Score /= Score(len(r))
	merged.Feedback = strings.Join(feedback, "\n\n")
	return merged, nil
}

// NewComposite snapshots evaluators. At least one non-nil evaluator is
// required.
func NewComposite[T any](evaluators ...Evaluator[T]) (*Composite[T], error) {
	if len(evaluators) == 0 {
		return nil, fmt.Errorf("%w: at least one evaluator is required", ErrInvalidConfig)
	}
	snapshot := make([]Evaluator[T], len(evaluators))
	for i, evaluator := range evaluators {
		if lo.IsNil(evaluator) {
			return nil, fmt.Errorf("%w: evaluators[%d] is nil", ErrInvalidConfig, i)
		}
		snapshot[i] = evaluator
	}
	return &Composite[T]{evaluators: snapshot}, nil
}

// Evaluate stops on the first child error or invalid child report. Error
// wrapping preserves errors.Is identities.
func (c *Composite[T]) Evaluate(ctx context.Context, subject T) (Report, error) {
	result := make(reports, 0, len(c.evaluators))
	for i, evaluator := range c.evaluators {
		if err := ctx.Err(); err != nil {
			return Report{}, err
		}
		report, err := evaluator.Evaluate(ctx, subject)
		if err != nil {
			return Report{}, fmt.Errorf("evaluation: evaluator %d: %w", i, err)
		}
		if err := report.Validate(); err != nil {
			return Report{}, fmt.Errorf("evaluation: evaluator %d: %w", i, err)
		}
		result = append(result, report.Clone())
	}
	return result.aggregate()
}
