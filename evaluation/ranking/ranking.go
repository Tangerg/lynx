// Package ranking evaluates ranked outputs against graded relevance judgments.
package ranking

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/Tangerg/scope/core/metadata"
	"github.com/Tangerg/scope/evaluation"
)

const (
	metricNamespace    = "ranking"
	metricCutoffKey    = "cutoff"
	metricThresholdKey = "threshold"
)

var ErrInvalidSample = errors.New("evaluation/ranking: invalid sample")

// Metric selects a ranking-quality calculation evaluated at a configured
// cutoff.
type Metric string

const (
	MetricPrecision        Metric = "precision"
	MetricRecall           Metric = "recall"
	MetricReciprocalRank   Metric = "reciprocal_rank"
	MetricAveragePrecision Metric = "average_precision"
	MetricNDCG             Metric = "ndcg"
	rankingField                  = "ranking"
	judgmentField                 = "judgments"
)

func (metric Metric) Validate() error {
	switch metric {
	case MetricPrecision, MetricRecall, MetricReciprocalRank, MetricAveragePrecision, MetricNDCG:
		return nil
	default:
		return fmt.Errorf("%w: unsupported ranking metric %q", evaluation.ErrInvalidMetric, metric)
	}
}

func (metric Metric) reportMetric(cutoff int, threshold *evaluation.Score) (evaluation.Metric, error) {
	parameters := metadata.Map{}
	if err := parameters.Set(metricCutoffKey, cutoff); err != nil {
		return evaluation.Metric{}, err
	}
	if threshold != nil {
		if err := parameters.Set(metricThresholdKey, threshold); err != nil {
			return evaluation.Metric{}, err
		}
	}
	return evaluation.NewMetric(evaluation.MetricConfig{
		Namespace: metricNamespace, Name: evaluation.MetricName(metric), Parameters: parameters,
	})
}

// Judgment assigns a non-negative relevance grade to one ranked identity. A
// positive grade is relevant for binary metrics; NDCG uses the full grade.
type Judgment struct {
	Identity  string  `json:"identity"`
	Relevance float64 `json:"relevance"`
}

// Sample contains one observed ranking and its relevance judgments. Ranking
// identities without a judgment are treated as having zero relevance.
type Sample struct {
	Ranking   []string   `json:"ranking,omitzero"`
	Judgments []Judgment `json:"judgments"`
}

func NewSample(ranking []string, judgments []Judgment) (Sample, error) {
	sample := Sample{Ranking: slices.Clone(ranking), Judgments: slices.Clone(judgments)}
	if err := sample.Validate(); err != nil {
		return Sample{}, err
	}
	return sample, nil
}

func (sample Sample) Clone() Sample {
	sample.Ranking = slices.Clone(sample.Ranking)
	sample.Judgments = slices.Clone(sample.Judgments)
	return sample
}

func (sample Sample) Validate() error {
	if err := validateIdentities(rankingField, sample.Ranking); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(sample.Judgments))
	hasRelevant := false
	for index, judgment := range sample.Judgments {
		if err := validateIdentity(fmt.Sprintf("%s[%d].identity", judgmentField, index), judgment.Identity); err != nil {
			return err
		}
		if _, exists := seen[judgment.Identity]; exists {
			return fmt.Errorf("%w: %s contains duplicate identity %q", ErrInvalidSample, judgmentField, judgment.Identity)
		}
		seen[judgment.Identity] = struct{}{}
		if math.IsNaN(judgment.Relevance) || math.IsInf(judgment.Relevance, 0) || judgment.Relevance < 0 {
			return fmt.Errorf("%w: %s[%d].relevance must be finite and non-negative", ErrInvalidSample, judgmentField, index)
		}
		hasRelevant = hasRelevant || judgment.Relevance > 0
	}
	if !hasRelevant {
		return fmt.Errorf("%w: at least one positive relevance judgment is required", ErrInvalidSample)
	}
	return nil
}

func validateIdentities(label string, identities []string) error {
	seen := make(map[string]struct{}, len(identities))
	for index, identity := range identities {
		if err := validateIdentity(fmt.Sprintf("%s[%d]", label, index), identity); err != nil {
			return err
		}
		if _, exists := seen[identity]; exists {
			return fmt.Errorf("%w: %s contains duplicate identity %q", ErrInvalidSample, label, identity)
		}
		seen[identity] = struct{}{}
	}
	return nil
}

func validateIdentity(label, identity string) error {
	if identity == "" || identity != strings.TrimSpace(identity) {
		return fmt.Errorf("%w: %s must be non-empty without surrounding whitespace", ErrInvalidSample, label)
	}
	return nil
}

type relevanceSet map[string]float64

func newRelevanceSet(judgments []Judgment) relevanceSet {
	relevance := make(relevanceSet, len(judgments))
	for _, judgment := range judgments {
		relevance[judgment.Identity] = judgment.Relevance
	}
	return relevance
}

func (relevance relevanceSet) relevantCount() int {
	count := 0
	for _, grade := range relevance {
		if grade > 0 {
			count++
		}
	}
	return count
}

func (relevance relevanceSet) count(ranking []string) int {
	count := 0
	for _, identity := range ranking {
		if relevance[identity] > 0 {
			count++
		}
	}
	return count
}

func (relevance relevanceSet) precisionAt(ranking []string, cutoff int) float64 {
	return float64(relevance.count(ranking)) / float64(cutoff)
}

func (relevance relevanceSet) recallAt(ranking []string) float64 {
	return float64(relevance.count(ranking)) / float64(relevance.relevantCount())
}

func (relevance relevanceSet) reciprocalRank(ranking []string) float64 {
	for index, identity := range ranking {
		if relevance[identity] > 0 {
			return 1 / float64(index+1)
		}
	}
	return 0
}

func (relevance relevanceSet) averagePrecisionAt(ranking []string, cutoff int) float64 {
	hits := 0
	total := 0.0
	for index, identity := range ranking {
		if relevance[identity] <= 0 {
			continue
		}
		hits++
		total += float64(hits) / float64(index+1)
	}
	return total / float64(min(relevance.relevantCount(), cutoff))
}

func discountedGain(relevance float64, rank int) float64 {
	return relevance / math.Log2(float64(rank+1))
}

func (relevance relevanceSet) ndcgAt(ranking []string, cutoff int) float64 {
	dcg := 0.0
	for index, identity := range ranking {
		dcg += discountedGain(relevance[identity], index+1)
	}
	ideal := make([]float64, 0, len(relevance))
	for _, grade := range relevance {
		if grade > 0 {
			ideal = append(ideal, grade)
		}
	}
	slices.SortFunc(ideal, func(left, right float64) int {
		if left > right {
			return -1
		}
		if left < right {
			return 1
		}
		return 0
	})
	idcg := 0.0
	for index, grade := range ideal[:min(cutoff, len(ideal))] {
		idcg += discountedGain(grade, index+1)
	}
	return dcg / idcg
}

func (metric Metric) score(ranking []string, relevance relevanceSet, cutoff int) float64 {
	switch metric {
	case MetricPrecision:
		return relevance.precisionAt(ranking, cutoff)
	case MetricRecall:
		return relevance.recallAt(ranking)
	case MetricReciprocalRank:
		return relevance.reciprocalRank(ranking)
	case MetricAveragePrecision:
		return relevance.averagePrecisionAt(ranking, cutoff)
	case MetricNDCG:
		return relevance.ndcgAt(ranking, cutoff)
	default:
		return 0
	}
}

// Config configures one deterministic ranking evaluator. Cutoff is required
// and positive. A nil Threshold produces a score without a pass/fail verdict.
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

func (config Config) threshold() (*evaluation.Score, error) {
	if config.Threshold == nil {
		return nil, nil
	}
	threshold := *config.Threshold
	if err := threshold.Validate(); err != nil {
		return nil, fmt.Errorf("%w: threshold: %w", evaluation.ErrInvalidEvaluatorConfig, err)
	}
	return &threshold, nil
}

// Evaluator measures one standard ranking metric at a fixed cutoff.
type Evaluator struct {
	metric       Metric
	cutoff       int
	threshold    *evaluation.Score
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
	reportMetric, err := config.Metric.reportMetric(config.Cutoff, threshold)
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

	relevance := newRelevanceSet(sample.Judgments)
	ranking := sample.Ranking[:min(evaluator.cutoff, len(sample.Ranking))]
	value := evaluator.metric.score(ranking, relevance, evaluator.cutoff)
	score, err := evaluation.NewScore(value)
	if err != nil {
		return evaluation.Report{}, fmt.Errorf("evaluation/ranking: calculate %s: %w", evaluator.reportMetric, err)
	}
	verdict := evaluation.VerdictUnspecified
	if evaluator.threshold != nil {
		verdict, err = score.Verdict(*evaluator.threshold)
		if err != nil {
			return evaluation.Report{}, fmt.Errorf("evaluation/ranking: verdict: %w", err)
		}
	}
	report := evaluation.Report{Metric: evaluator.reportMetric.Clone(), Verdict: verdict, Score: &score}
	if err := report.Validate(); err != nil {
		return evaluation.Report{}, err
	}
	return report, nil
}

var _ evaluation.Evaluator[Sample] = (*Evaluator)(nil)
