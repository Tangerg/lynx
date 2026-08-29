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

// Distribution summarizes one homogeneous numeric signal. Count distinguishes
// an absent distribution from a real distribution whose values are all zero.
type Distribution struct {
	Count   int
	Mean    float64
	Minimum float64
	P10     float64
	P50     float64
	P90     float64
	Maximum float64
}

// MetricSummary keeps score and measurement distributions attached to their
// full Metric identity so unrelated units, directions, and configurations are
// never aggregated together. Runner summarizes both top-level reports and
// their Details.
type MetricSummary struct {
	Metric       Metric
	Evaluated    int
	Passed       int
	Failed       int
	Unjudged     int
	Scores       Distribution
	Measurements Distribution
}

type Summary struct {
	Total     int
	Evaluated int
	Passed    int
	Failed    int
	Unjudged  int
	Errors    int
	Metrics   []MetricSummary
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
	report.Summary.Metrics = slices.Clone(report.Summary.Metrics)
	for index := range report.Summary.Metrics {
		report.Summary.Metrics[index].Metric = report.Summary.Metrics[index].Metric.Clone()
	}
	return report
}

func (runner *Runner[T]) Run(ctx context.Context, cases []Case[T]) (RunReport, error) {
	if err := validateCases(cases); err != nil {
		return RunReport{}, err
	}
	results := newCaseReports(cases)
	if len(cases) == 0 {
		return RunReport{Cases: results}, nil
	}

	attempted, runErr := runner.execute(ctx, cases, results)
	markUnevaluated(results, attempted, ctx.Err())
	summary, summaryErr := summarize(results)
	report := RunReport{Cases: results, Summary: summary}
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

func newCaseReports[T any](cases []Case[T]) []CaseReport {
	results := make([]CaseReport, len(cases))
	for index, caseValue := range cases {
		results[index] = CaseReport{ID: caseValue.ID, Metadata: caseValue.Metadata.Clone()}
	}
	return results
}

func (runner *Runner[T]) execute(ctx context.Context, cases []Case[T], results []CaseReport) ([]bool, error) {
	group, groupContext := errgroup.WithContext(ctx)
	workerCount := min(runner.maxConcurrency, len(cases))
	var next atomic.Uint64
	attempted := make([]bool, len(cases))
	for range workerCount {
		group.Go(func() error { return runner.work(groupContext, cases, results, attempted, &next) })
	}
	return attempted, group.Wait()
}

func (runner *Runner[T]) work(
	ctx context.Context,
	cases []Case[T],
	results []CaseReport,
	attempted []bool,
	next *atomic.Uint64,
) error {
	for ctx.Err() == nil {
		index := int(next.Add(1) - 1)
		if index >= len(cases) || ctx.Err() != nil {
			return nil
		}
		attempted[index] = true
		caseValue := cases[index]
		report, err := runner.evaluator.Evaluate(ctx, caseValue.Subject)
		if err == nil {
			err = report.Validate()
		}
		if err == nil {
			results[index].Report = report.Clone()
			continue
		}
		results[index].Err = err
		if runner.errorPolicy == ErrorFailFast {
			return fmt.Errorf("evaluation: case %q: %w", caseValue.ID, err)
		}
	}
	return nil
}

func markUnevaluated(results []CaseReport, attempted []bool, contextErr error) {
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

func summarize(results []CaseReport) (Summary, error) {
	summary := Summary{Total: len(results)}
	type accumulator struct {
		index        int
		scores       []float64
		measurements []float64
	}
	metrics := make(map[string]*accumulator)
	var summarizeMetric func(Report) error
	summarizeMetric = func(report Report) error {
		identity, err := report.Metric.identity()
		if err != nil {
			return err
		}
		current := metrics[identity]
		if current == nil {
			current = &accumulator{index: len(summary.Metrics)}
			metrics[identity] = current
			summary.Metrics = append(summary.Metrics, MetricSummary{Metric: report.Metric.Clone()})
		}
		metricSummary := &summary.Metrics[current.index]
		metricSummary.Evaluated++
		switch report.Verdict {
		case VerdictPass:
			metricSummary.Passed++
		case VerdictFail:
			metricSummary.Failed++
		default:
			metricSummary.Unjudged++
		}
		if report.Score != nil {
			current.scores = append(current.scores, report.Score.Float64())
		}
		if report.Measurement != nil {
			current.measurements = append(current.measurements, *report.Measurement)
		}
		for _, detail := range report.Details {
			if err := summarizeMetric(detail); err != nil {
				return err
			}
		}
		return nil
	}
	for _, result := range results {
		if result.Err != nil {
			summary.Errors++
			continue
		}
		summary.Evaluated++
		switch result.Report.Verdict {
		case VerdictPass:
			summary.Passed++
		case VerdictFail:
			summary.Failed++
		default:
			summary.Unjudged++
		}

		if err := summarizeMetric(result.Report); err != nil {
			return Summary{}, fmt.Errorf("evaluation: summarize case %q: %w", result.ID, err)
		}
	}
	for _, current := range metrics {
		metricSummary := &summary.Metrics[current.index]
		metricSummary.Scores = distribution(current.scores)
		metricSummary.Measurements = distribution(current.measurements)
	}
	return summary, nil
}

func distribution(values []float64) Distribution {
	if len(values) == 0 {
		return Distribution{}
	}
	slices.Sort(values)
	result := Distribution{
		Count: len(values), Minimum: values[0], Maximum: values[len(values)-1],
		P10: percentile(values, 0.10), P50: percentile(values, 0.50), P90: percentile(values, 0.90),
	}
	for _, value := range values {
		result.Mean += value
	}
	result.Mean /= float64(len(values))
	return result
}

func percentile(sorted []float64, quantile float64) float64 {
	index := max(0, int(math.Ceil(quantile*float64(len(sorted))))-1)
	return sorted[index]
}
