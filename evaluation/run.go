package evaluation

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/samber/lo"
	"golang.org/x/sync/errgroup"

	"github.com/Tangerg/scope/core/metadata"
)

type ErrorPolicy string

const (
	ErrorCollect  ErrorPolicy = "collect"
	ErrorFailFast ErrorPolicy = "fail_fast"

	DefaultMaxConcurrency = 4
)

// Case gives a stable identity to one subject in an evaluation run.
type Case[T any] struct {
	ID       string
	Subject  T
	Metadata metadata.Map
}

func NewCase[T any](id string, subject T, caseMetadata metadata.Map) (Case[T], error) {
	caseValue := Case[T]{ID: id, Subject: subject, Metadata: caseMetadata.Clone()}
	if err := caseValue.Validate(); err != nil {
		return Case[T]{}, err
	}
	return caseValue, nil
}

func (caseValue Case[T]) Validate() error {
	if caseValue.ID == "" || caseValue.ID != strings.TrimSpace(caseValue.ID) {
		return fmt.Errorf("%w: id must be non-empty without surrounding whitespace", ErrInvalidCase)
	}
	if err := caseValue.Metadata.Validate(); err != nil {
		return fmt.Errorf("%w: metadata: %w", ErrInvalidCase, err)
	}
	return nil
}

type RunnerConfig struct {
	MaxConcurrency int
	ErrorPolicy    ErrorPolicy
}

type Runner[T any] struct {
	evaluator      Evaluator[T]
	maxConcurrency int
	errorPolicy    ErrorPolicy
}

func NewRunner[T any](evaluator Evaluator[T], config RunnerConfig) (*Runner[T], error) {
	if lo.IsNil(evaluator) {
		return nil, fmt.Errorf("%w: evaluator is nil", ErrInvalidRunConfig)
	}
	if config.MaxConcurrency < 0 {
		return nil, fmt.Errorf("%w: maximum concurrency must not be negative", ErrInvalidRunConfig)
	}
	maxConcurrency := config.MaxConcurrency
	if maxConcurrency == 0 {
		maxConcurrency = DefaultMaxConcurrency
	}
	policy := config.ErrorPolicy
	if policy == "" {
		policy = ErrorCollect
	}
	if policy != ErrorCollect && policy != ErrorFailFast {
		return nil, fmt.Errorf("%w: unsupported error policy %q", ErrInvalidRunConfig, policy)
	}
	return &Runner[T]{evaluator: evaluator, maxConcurrency: maxConcurrency, errorPolicy: policy}, nil
}

type CaseReport struct {
	ID       string
	Metadata metadata.Map
	Report   Report
	Err      error
}

func (report CaseReport) Clone() CaseReport {
	report.Metadata = report.Metadata.Clone()
	report.Report = report.Report.Clone()
	return report
}

type Summary struct {
	Total     int
	Evaluated int
	Passed    int
	Failed    int
	Errors    int
	Mean      Score
	Minimum   Score
	P10       Score
	P50       Score
	P90       Score
	Maximum   Score
}

type RunReport struct {
	Cases   []CaseReport
	Summary Summary
}

func (report RunReport) Clone() RunReport {
	report.Cases = slices.Clone(report.Cases)
	for index := range report.Cases {
		report.Cases[index] = report.Cases[index].Clone()
	}
	return report
}

func (runner *Runner[T]) Run(ctx context.Context, cases []Case[T]) (RunReport, error) {
	if err := validateCases(cases); err != nil {
		return RunReport{}, err
	}
	results := make([]CaseReport, len(cases))
	for index, caseValue := range cases {
		results[index] = CaseReport{ID: caseValue.ID, Metadata: caseValue.Metadata.Clone()}
	}
	if len(cases) == 0 {
		return RunReport{Cases: results}, nil
	}

	group, groupContext := errgroup.WithContext(ctx)
	workerCount := min(runner.maxConcurrency, len(cases))
	var next atomic.Uint64
	attempted := make([]bool, len(cases))
	for range workerCount {
		group.Go(func() error {
			for {
				if groupContext.Err() != nil {
					return nil
				}
				index := int(next.Add(1) - 1)
				if index >= len(cases) {
					return nil
				}
				if groupContext.Err() != nil {
					return nil
				}
				attempted[index] = true
				caseValue := cases[index]
				report, err := runner.evaluator.Evaluate(groupContext, caseValue.Subject)
				if err == nil {
					err = report.Validate()
				}
				if err != nil {
					results[index].Err = err
					if runner.errorPolicy == ErrorFailFast {
						return fmt.Errorf("evaluation: case %q: %w", caseValue.ID, err)
					}
					continue
				}
				results[index].Report = report.Clone()
			}
		})
	}
	err := group.Wait()
	pendingErr := context.Canceled
	if ctx.Err() != nil {
		pendingErr = ctx.Err()
	}
	for index := range results {
		if !attempted[index] {
			results[index].Err = pendingErr
		}
	}
	report := RunReport{Cases: results, Summary: summarize(results)}
	if err != nil {
		return report, err
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	return report, nil
}

func validateCases[T any](cases []Case[T]) error {
	seen := make(map[string]struct{}, len(cases))
	for index, caseValue := range cases {
		if err := caseValue.Validate(); err != nil {
			return fmt.Errorf("%w: cases[%d]: %w", ErrInvalidCase, index, err)
		}
		if _, exists := seen[caseValue.ID]; exists {
			return fmt.Errorf("%w: duplicate id %q", ErrInvalidCase, caseValue.ID)
		}
		seen[caseValue.ID] = struct{}{}
	}
	return nil
}

func summarize(results []CaseReport) Summary {
	summary := Summary{Total: len(results)}
	scores := make([]Score, 0, len(results))
	for _, result := range results {
		if result.Err != nil {
			summary.Errors++
			continue
		}
		if err := result.Report.Validate(); err != nil {
			summary.Errors++
			continue
		}
		summary.Evaluated++
		if result.Report.Passed {
			summary.Passed++
		} else {
			summary.Failed++
		}
		scores = append(scores, result.Report.Score)
		summary.Mean += result.Report.Score
	}
	if len(scores) == 0 {
		return summary
	}
	summary.Mean /= Score(len(scores))
	slices.Sort(scores)
	summary.Minimum = scores[0]
	summary.P10 = percentile(scores, 0.10)
	summary.P50 = percentile(scores, 0.50)
	summary.P90 = percentile(scores, 0.90)
	summary.Maximum = scores[len(scores)-1]
	return summary
}

func percentile(sorted []Score, quantile float64) Score {
	index := max(0, int(math.Ceil(quantile*float64(len(sorted))))-1)
	return sorted[index]
}
