package evaluation

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
)

// RetrievalMetric identifies a ranking-quality calculation evaluated at a
// configured cutoff.
type RetrievalMetric string

const (
	RetrievalMetricPrecision      RetrievalMetric = "precision"
	RetrievalMetricRecall         RetrievalMetric = "recall"
	RetrievalMetricReciprocalRank RetrievalMetric = "reciprocal_rank"
	RetrievalMetricNDCG           RetrievalMetric = "ndcg"
)

func (metric RetrievalMetric) Validate() error {
	switch metric {
	case RetrievalMetricPrecision, RetrievalMetricRecall, RetrievalMetricReciprocalRank, RetrievalMetricNDCG:
		return nil
	default:
		return fmt.Errorf("%w: unsupported retrieval metric %q", ErrInvalidMetric, metric)
	}
}

func (metric RetrievalMetric) reportMetric(cutoff int) (Metric, error) {
	reportMetric := Metric(fmt.Sprintf("retrieval/%s@%d", metric, cutoff))
	if err := reportMetric.Validate(); err != nil {
		return "", err
	}
	return reportMetric, nil
}

// RetrievalSample is an observed ranking and its complete binary relevance
// judgment. Identifiers are provider-neutral: callers may use document IDs,
// canonical URLs, chunk IDs, or another stable identity.
type RetrievalSample struct {
	Retrieved []string `json:"retrieved,omitzero"`
	Relevant  []string `json:"relevant"`
}

func NewRetrievalSample(retrieved, relevant []string) (RetrievalSample, error) {
	sample := RetrievalSample{
		Retrieved: slices.Clone(retrieved),
		Relevant:  slices.Clone(relevant),
	}
	if err := sample.Validate(); err != nil {
		return RetrievalSample{}, err
	}
	return sample, nil
}

func (sample RetrievalSample) Clone() RetrievalSample {
	sample.Retrieved = slices.Clone(sample.Retrieved)
	sample.Relevant = slices.Clone(sample.Relevant)
	return sample
}

func (sample RetrievalSample) Validate() error {
	if len(sample.Relevant) == 0 {
		return fmt.Errorf("%w: at least one relevant identity is required", ErrInvalidSample)
	}
	if err := identityList(sample.Retrieved).validate("retrieved"); err != nil {
		return err
	}
	return identityList(sample.Relevant).validate("relevant")
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

// RetrievalEvaluatorConfig configures a deterministic retrieval evaluator.
// Cutoff is required and positive; a nil Threshold selects [DefaultThreshold].
type RetrievalEvaluatorConfig struct {
	Metric    RetrievalMetric
	Cutoff    int
	Threshold *Score
}

func (config RetrievalEvaluatorConfig) validate() error {
	if err := config.Metric.Validate(); err != nil {
		return fmt.Errorf("%w: metric: %w", ErrInvalidEvaluatorConfig, err)
	}
	if config.Cutoff <= 0 {
		return fmt.Errorf("%w: cutoff must be positive", ErrInvalidEvaluatorConfig)
	}
	return nil
}

func (config RetrievalEvaluatorConfig) threshold() (Score, error) {
	threshold, err := config.Threshold.valueOr(DefaultThreshold)
	if err != nil {
		return 0, fmt.Errorf("%w: threshold: %w", ErrInvalidEvaluatorConfig, err)
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

func (relevant relevanceSet) ndcgAt(ranking []string, cutoff int) float64 {
	dcg := 0.0
	for index, identity := range ranking {
		if _, found := relevant[identity]; found {
			dcg += 1 / math.Log2(float64(index+2))
		}
	}
	idcg := 0.0
	for index := range min(cutoff, len(relevant)) {
		idcg += 1 / math.Log2(float64(index+2))
	}
	return dcg / idcg
}

func (metric RetrievalMetric) score(ranking []string, relevant relevanceSet, cutoff int) float64 {
	switch metric {
	case RetrievalMetricPrecision:
		return relevant.precisionAt(ranking, cutoff)
	case RetrievalMetricRecall:
		return float64(relevant.count(ranking)) / float64(len(relevant))
	case RetrievalMetricReciprocalRank:
		return relevant.reciprocalRank(ranking)
	case RetrievalMetricNDCG:
		return relevant.ndcgAt(ranking, cutoff)
	default:
		return 0
	}
}

// RetrievalEvaluator measures one standard ranking metric at a fixed cutoff.
type RetrievalEvaluator struct {
	metric       RetrievalMetric
	cutoff       int
	threshold    Score
	reportMetric Metric
}

func NewRetrievalEvaluator(config RetrievalEvaluatorConfig) (*RetrievalEvaluator, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	threshold, err := config.threshold()
	if err != nil {
		return nil, err
	}
	reportMetric, err := config.Metric.reportMetric(config.Cutoff)
	if err != nil {
		return nil, fmt.Errorf("%w: report metric: %w", ErrInvalidEvaluatorConfig, err)
	}
	return &RetrievalEvaluator{
		metric:       config.Metric,
		cutoff:       config.Cutoff,
		threshold:    threshold,
		reportMetric: reportMetric,
	}, nil
}

func (evaluator *RetrievalEvaluator) Evaluate(ctx context.Context, sample RetrievalSample) (Report, error) {
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	if err := sample.Validate(); err != nil {
		return Report{}, err
	}

	relevant := newRelevanceSet(sample.Relevant)
	ranking := sample.Retrieved[:min(evaluator.cutoff, len(sample.Retrieved))]

	value := evaluator.metric.score(ranking, relevant, evaluator.cutoff)
	score, err := NewScore(value)
	if err != nil {
		return Report{}, fmt.Errorf("evaluation: calculate %s: %w", evaluator.reportMetric, err)
	}
	report := Report{Metric: evaluator.reportMetric, Passed: score.Passes(evaluator.threshold), Score: score}
	if err := report.Validate(); err != nil {
		return Report{}, err
	}
	return report, nil
}
