package evaluation

import (
	"context"
	"fmt"

	"github.com/samber/lo"
)

const MetricNameSuite MetricName = "suite"

// SuiteConfig groups heterogeneous evaluators without collapsing their
// results into one score. A zero MaxConcurrency selects
// DefaultMaxConcurrency.
type SuiteConfig[T any] struct {
	Evaluators     []Evaluator[T]
	MaxConcurrency int
}

// SuiteEvaluator preserves every child report. Its verdict fails when any
// decided child fails, passes when at least one child passes and none fail, and
// remains unspecified when every child is measurement-only or qualitative.
type SuiteEvaluator[T any] struct {
	evaluators     []Evaluator[T]
	maxConcurrency int
	metric         Metric
}

func NewSuiteEvaluator[T any](config SuiteConfig[T]) (*SuiteEvaluator[T], error) {
	if len(config.Evaluators) == 0 {
		return nil, fmt.Errorf("%w: at least one evaluator is required", ErrInvalidEvaluatorConfig)
	}
	if config.MaxConcurrency < 0 {
		return nil, fmt.Errorf("%w: maximum concurrency must not be negative", ErrInvalidEvaluatorConfig)
	}
	evaluators := make([]Evaluator[T], len(config.Evaluators))
	for index, evaluator := range config.Evaluators {
		if lo.IsNil(evaluator) {
			return nil, fmt.Errorf("%w: evaluators[%d] is nil", ErrInvalidEvaluatorConfig, index)
		}
		evaluators[index] = evaluator
	}
	maxConcurrency := config.MaxConcurrency
	if maxConcurrency == 0 {
		maxConcurrency = DefaultMaxConcurrency
	}
	maxConcurrency = min(maxConcurrency, len(evaluators))
	metric, err := NewMetric(MetricConfig{Name: MetricNameSuite})
	if err != nil {
		return nil, fmt.Errorf("%w: suite metric: %w", ErrInvalidEvaluatorConfig, err)
	}
	return &SuiteEvaluator[T]{evaluators: evaluators, maxConcurrency: maxConcurrency, metric: metric}, nil
}

func (suite *SuiteEvaluator[T]) Evaluate(ctx context.Context, subject T) (Report, error) {
	reports, err := evaluateAll(ctx, suite.evaluators, suite.maxConcurrency, subject)
	if err != nil {
		return Report{}, err
	}
	verdict := VerdictUnspecified
	for _, report := range reports {
		switch report.Verdict {
		case VerdictFail:
			verdict = VerdictFail
		case VerdictPass:
			if verdict == VerdictUnspecified {
				verdict = VerdictPass
			}
		}
	}
	result := Report{Metric: suite.metric.Clone(), Verdict: verdict, Details: reports}
	if err := result.Validate(); err != nil {
		return Report{}, err
	}
	return result, nil
}

var _ Evaluator[struct{}] = (*SuiteEvaluator[struct{}])(nil)
