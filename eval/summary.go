package eval

import (
	"fmt"
	"math"
	"slices"

	"github.com/Tangerg/scope/core/metadata"
)

type CaseResult struct {
	ID       CaseID
	Metadata metadata.Map
	Report   Report
	Err      error
}

func (result CaseResult) clone() CaseResult {
	result.Metadata = result.Metadata.Clone()
	if result.Err == nil {
		result.Report = result.Report.cloneValid()
	}
	return result
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
// never aggregated together. Experiment summarizes both top-level reports and
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

type ExperimentSummary struct {
	Total     int
	Evaluated int
	Passed    int
	Failed    int
	Unjudged  int
	Errors    int
	Metrics   []MetricSummary
}

type ExperimentReport struct {
	cases   []CaseResult
	summary ExperimentSummary
}

// Cases returns owned results in Dataset order.
func (report ExperimentReport) Cases() []CaseResult {
	return cloneCaseResults(report.cases)
}

// Summary returns the owned aggregate calculated from Cases.
func (report ExperimentReport) Summary() ExperimentSummary {
	return cloneExperimentSummary(report.summary)
}

func newExperimentReport(cases []CaseResult, summary ExperimentSummary) ExperimentReport {
	return ExperimentReport{
		cases: cloneCaseResults(cases), summary: cloneExperimentSummary(summary),
	}
}

func cloneCaseResults(results []CaseResult) []CaseResult {
	cloned := slices.Clone(results)
	for index := range cloned {
		cloned[index] = cloned[index].clone()
	}
	return cloned
}

func cloneExperimentSummary(summary ExperimentSummary) ExperimentSummary {
	summary.Metrics = cloneMetricSummaries(summary.Metrics)
	return summary
}

func cloneMetricSummaries(summaries []MetricSummary) []MetricSummary {
	cloned := slices.Clone(summaries)
	for index := range cloned {
		cloned[index] = cloneMetricSummary(cloned[index])
	}
	return cloned
}

func cloneMetricSummary(summary MetricSummary) MetricSummary {
	summary.Metric = summary.Metric.Clone()
	return summary
}

func newCaseResults[T any](cases []Case[T]) []CaseResult {
	results := make([]CaseResult, len(cases))
	for index, caseValue := range cases {
		results[index] = CaseResult{ID: caseValue.ID, Metadata: caseValue.Metadata.Clone()}
	}
	return results
}

func summarize(results []CaseResult) (ExperimentSummary, error) {
	summary := ExperimentSummary{Total: len(results)}
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
		if err := result.Report.Validate(); err != nil {
			return ExperimentSummary{}, fmt.Errorf("eval: summarize case %q: %w", result.ID, err)
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
			return ExperimentSummary{}, fmt.Errorf("eval: summarize case %q: %w", result.ID, err)
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
