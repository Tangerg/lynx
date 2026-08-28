// Package retrieval evaluates provider-neutral ranked retrieval results.
package retrieval

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/Tangerg/scope/evaluation"
)

var ErrInvalidSample = errors.New("evaluation/retrieval: invalid sample")

// Metric identifies a ranking-quality calculation evaluated at a configured
// cutoff.
type Metric string

const (
	MetricPrecision      Metric = "precision"
	MetricRecall         Metric = "recall"
	MetricReciprocalRank Metric = "reciprocal_rank"
	MetricNDCG           Metric = "ndcg"
	reportMetricFormat          = "retrieval/%s@%d"
	retrievedField              = "retrieved"
	relevantField               = "relevant"
)

func (metric Metric) Validate() error {
	switch metric {
	case MetricPrecision, MetricRecall, MetricReciprocalRank, MetricNDCG:
		return nil
	default:
		return fmt.Errorf("%w: unsupported retrieval metric %q", evaluation.ErrInvalidMetric, metric)
	}
}

func (metric Metric) reportMetric(cutoff int) (evaluation.Metric, error) {
	reportMetric := evaluation.Metric(fmt.Sprintf(reportMetricFormat, metric, cutoff))
	if err := reportMetric.Validate(); err != nil {
		return "", err
	}
	return reportMetric, nil
}

// Sample is an observed ranking and its complete binary relevance judgment.
// Identifiers are provider-neutral: callers may use document IDs, canonical
// URLs, chunk IDs, or another stable identity.
type Sample struct {
	Retrieved []string `json:"retrieved,omitzero"`
	Relevant  []string `json:"relevant"`
}

func NewSample(retrieved, relevant []string) (Sample, error) {
	sample := Sample{Retrieved: slices.Clone(retrieved), Relevant: slices.Clone(relevant)}
	if err := sample.Validate(); err != nil {
		return Sample{}, err
	}
	return sample, nil
}

func (sample Sample) Clone() Sample {
	sample.Retrieved = slices.Clone(sample.Retrieved)
	sample.Relevant = slices.Clone(sample.Relevant)
	return sample
}

func (sample Sample) Validate() error {
	if len(sample.Relevant) == 0 {
		return fmt.Errorf("%w: at least one relevant identity is required", ErrInvalidSample)
	}
	if err := identityList(sample.Retrieved).validate(retrievedField); err != nil {
		return err
	}
	return identityList(sample.Relevant).validate(relevantField)
}

type identityList []string

func (identities identityList) validate(label string) error {
	seen := make(map[string]struct{}, len(identities))
	for index, id := range identities {
		if id == "" || id != strings.TrimSpace(id) {
			return fmt.Errorf("%w: %s[%d] must be non-empty without surrounding whitespace", ErrInvalidSample, label, index)
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("%w: %s contains duplicate identity %q", ErrInvalidSample, label, id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

// Config configures one deterministic retrieval evaluator. Cutoff is required
// and positive; a nil Threshold selects [evaluation.DefaultThreshold].
type Config struct {
	Metric    Metric
	Cutoff    int
	Threshold *evaluation.Score
}

func (config Config) validate() error {
	if err := config.Metric.Validate(); err != nil {
		return fmt.Errorf("%w: metric: %w", evaluation.ErrInvalidEvaluatorConfig, err)
	}
	if config.Cutoff <= 0 {
		return fmt.Errorf("%w: cutoff must be positive", evaluation.ErrInvalidEvaluatorConfig)
	}
	return nil
}

func (config Config) threshold() (evaluation.Score, error) {
	threshold := evaluation.DefaultThreshold
	if config.Threshold != nil {
		threshold = *config.Threshold
	}
	if err := threshold.Validate(); err != nil {
		return 0, fmt.Errorf("%w: threshold: %w", evaluation.ErrInvalidEvaluatorConfig, err)
	}
	return threshold, nil
}

type relevanceSet map[string]struct{}

func newRelevanceSet(identities []string) relevanceSet {
	relevant := make(relevanceSet, len(identities))
	for _, identity := range identities {
		relevant[identity] = struct{}{}
	}
	return relevant
}

func (relevant relevanceSet) count(ranking []string) int {
	count := 0
	for _, identity := range ranking {
		if _, found := relevant[identity]; found {
			count++
		}
	}
	return count
}

func (relevant relevanceSet) precisionAt(ranking []string, cutoff int) float64 {
	return float64(relevant.count(ranking)) / float64(cutoff)
}

func (relevant relevanceSet) reciprocalRank(ranking []string) float64 {
	for index, identity := range ranking {
		if _, found := relevant[identity]; found {
			return 1 / float64(index+1)
		}
	}
	return 0
}

func discountedGain(rank int) float64 {
	return 1 / math.Log2(float64(rank+1))
}

func (relevant relevanceSet) ndcgAt(ranking []string, cutoff int) float64 {
	dcg := 0.0
	for index, identity := range ranking {
		if _, found := relevant[identity]; found {
			dcg += discountedGain(index + 1)
		}
	}
	idcg := 0.0
	for index := range min(cutoff, len(relevant)) {
		idcg += discountedGain(index + 1)
	}
	return dcg / idcg
}

func (metric Metric) score(ranking []string, relevant relevanceSet, cutoff int) float64 {
	switch metric {
	case MetricPrecision:
		return relevant.precisionAt(ranking, cutoff)
	case MetricRecall:
		return float64(relevant.count(ranking)) / float64(len(relevant))
	case MetricReciprocalRank:
		return relevant.reciprocalRank(ranking)
	case MetricNDCG:
		return relevant.ndcgAt(ranking, cutoff)
	default:
		return 0
	}
}

// Evaluator measures one standard ranking metric at a fixed cutoff.
type Evaluator struct {
	metric       Metric
	cutoff       int
	threshold    evaluation.Score
	reportMetric evaluation.Metric
}

func NewEvaluator(config Config) (*Evaluator, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	threshold, err := config.threshold()
	if err != nil {
		return nil, err
	}
	reportMetric, err := config.Metric.reportMetric(config.Cutoff)
	if err != nil {
		return nil, fmt.Errorf("%w: report metric: %w", evaluation.ErrInvalidEvaluatorConfig, err)
	}
	return &Evaluator{
		metric: config.Metric, cutoff: config.Cutoff, threshold: threshold, reportMetric: reportMetric,
	}, nil
}

func (evaluator *Evaluator) Evaluate(ctx context.Context, sample Sample) (evaluation.Report, error) {
	if err := ctx.Err(); err != nil {
		return evaluation.Report{}, err
	}
	if err := sample.Validate(); err != nil {
		return evaluation.Report{}, err
	}

	relevant := newRelevanceSet(sample.Relevant)
	ranking := sample.Retrieved[:min(evaluator.cutoff, len(sample.Retrieved))]
	value := evaluator.metric.score(ranking, relevant, evaluator.cutoff)
	score, err := evaluation.NewScore(value)
	if err != nil {
		return evaluation.Report{}, fmt.Errorf("evaluation/retrieval: calculate %s: %w", evaluator.reportMetric, err)
	}
	report := evaluation.Report{
		Metric: evaluator.reportMetric, Passed: score.Passes(evaluator.threshold), Score: score,
	}
	if err := report.Validate(); err != nil {
		return evaluation.Report{}, err
	}
	return report, nil
}

var _ evaluation.Evaluator[Sample] = (*Evaluator)(nil)
