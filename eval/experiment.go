package eval

import (
	"context"
	"fmt"

	"github.com/samber/lo"
	"golang.org/x/sync/errgroup"
)

const DefaultMaxConcurrency = 4

type ErrorPolicy string

const (
	ErrorCollect  ErrorPolicy = "collect"
	ErrorFailFast ErrorPolicy = "fail_fast"
)

func (policy ErrorPolicy) normalized() (ErrorPolicy, error) {
	if policy == "" {
		return ErrorCollect, nil
	}
	switch policy {
	case ErrorCollect, ErrorFailFast:
		return policy, nil
	default:
		return "", fmt.Errorf("%w: unsupported error policy %q", ErrInvalidExperiment, policy)
	}
}

type ExperimentConfig[T any] struct {
	Dataset        Dataset[T]
	Evaluator      Evaluator[T]
	MaxConcurrency int
	ErrorPolicy    ErrorPolicy
}

// Experiment is an immutable plan for evaluating one Dataset. It owns bounded
// scheduling and error semantics, but no persistence, artifacts, or product
// identity.
type Experiment[T any] struct {
	dataset        Dataset[T]
	evaluator      Evaluator[T]
	maxConcurrency int
	errorPolicy    ErrorPolicy
}

func NewExperiment[T any](config ExperimentConfig[T]) (Experiment[T], error) {
	if lo.IsNil(config.Evaluator) {
		return Experiment[T]{}, fmt.Errorf("%w: evaluator is nil", ErrInvalidExperiment)
	}
	if config.MaxConcurrency < 0 {
		return Experiment[T]{}, fmt.Errorf("%w: maximum concurrency must not be negative", ErrInvalidExperiment)
	}
	policy, err := config.ErrorPolicy.normalized()
	if err != nil {
		return Experiment[T]{}, err
	}
	maxConcurrency := config.MaxConcurrency
	if maxConcurrency == 0 {
		maxConcurrency = DefaultMaxConcurrency
	}
	dataset, err := NewDataset(config.Dataset.Cases()...)
	if err != nil {
		return Experiment[T]{}, fmt.Errorf("%w: dataset: %w", ErrInvalidExperiment, err)
	}
	return Experiment[T]{
		dataset: dataset, evaluator: config.Evaluator,
		maxConcurrency: maxConcurrency, errorPolicy: policy,
	}, nil
}

func (experiment Experiment[T]) Run(ctx context.Context) (ExperimentReport, error) {
	cases := experiment.dataset.Cases()
	results := newCaseResults(cases)
	if len(cases) == 0 {
		return newExperimentReport(results, ExperimentSummary{}), nil
	}

	attempted, runErr := experiment.execute(ctx, cases, results)
	markUnevaluated(results, attempted, ctx.Err())
	summary, summaryErr := summarize(results)
	report := newExperimentReport(results, summary)
	if runErr != nil {
		return report, runErr
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	if summaryErr != nil {
		return report, summaryErr
	}
	return report, nil
}

func (experiment Experiment[T]) execute(
	ctx context.Context,
	cases []Case[T],
	results []CaseResult,
) ([]bool, error) {
	group, groupContext := errgroup.WithContext(ctx)
	group.SetLimit(min(experiment.maxConcurrency, len(cases)))
	attempted := make([]bool, len(cases))
	for index, caseValue := range cases {
		if groupContext.Err() != nil {
			break
		}
		group.Go(func() error {
			if groupContext.Err() != nil {
				return nil
			}
			attempted[index] = true
			report, err := experiment.evaluator.Evaluate(groupContext, caseValue.Subject)
			if err == nil {
				err = report.Validate()
			}
			if err == nil {
				results[index].Report = report.cloneValid()
				return nil
			}
			results[index].Err = err
			if experiment.errorPolicy == ErrorFailFast {
				return fmt.Errorf("eval: case %q: %w", caseValue.ID, err)
			}
			return nil
		})
	}
	return attempted, group.Wait()
}

func markUnevaluated(results []CaseResult, attempted []bool, contextErr error) {
	pendingErr := ErrCaseNotEvaluated
	if contextErr != nil {
		pendingErr = contextErr
	}
	for index := range results {
		if !attempted[index] {
			results[index].Err = pendingErr
		}
	}
}
