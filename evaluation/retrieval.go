package evaluation

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
)

// RetrievalMetric identifies a ranking-quality calculation. Each metric is
// evaluated at the configured cutoff K.
type RetrievalMetric string

const (
	RetrievalPrecision      RetrievalMetric = "precision"
	RetrievalRecall         RetrievalMetric = "recall"
	RetrievalReciprocalRank RetrievalMetric = "reciprocal_rank"
	RetrievalNDCG           RetrievalMetric = "ndcg"
)

// Validate reports whether the metric is supported.
func (m RetrievalMetric) Validate() error {
	switch m {
	case RetrievalPrecision, RetrievalRecall, RetrievalReciprocalRank, RetrievalNDCG:
		return nil
	default:
		return fmt.Errorf("unsupported retrieval metric %q", m)
	}
}

func (m RetrievalMetric) reportMetric(k int) (Metric, error) {
	return NewMetric(fmt.Sprintf("retrieval/%s@%d", m, k))
}

// RetrievalSample is an observed ranking and its complete binary relevance
// judgment. Identifiers are provider-neutral: callers may use document IDs,
// canonical URLs, chunk IDs, or another stable identity.
type RetrievalSample struct {
	Retrieved []string `json:"retrieved,omitzero"`
	Relevant  []string `json:"relevant"`
}

// NewRetrievalSample validates and snapshots a retrieval evaluation input.
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

// Clone returns an independent copy of the sample.
func (r RetrievalSample) Clone() RetrievalSample {
	r.Retrieved = slices.Clone(r.Retrieved)
	r.Relevant = slices.Clone(r.Relevant)
	return r
}

// Validate checks the ranking identities and relevance judgment. Duplicate
// identities are rejected because ranking metrics assume one position per
// item and should expose duplicate retrieval bugs instead of hiding them.
func (r RetrievalSample) Validate() error {
	if len(r.Relevant) == 0 {
		return fmt.Errorf("%w: at least one relevant identity is required", ErrInvalidSample)
	}
	if err := validateRetrievalIDs("retrieved", r.Retrieved); err != nil {
		return err
	}
	return validateRetrievalIDs("relevant", r.Relevant)
}

func validateRetrievalIDs(label string, ids []string) error {
	seen := make(map[string]struct{}, len(ids))
	for index, id := range ids {
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

// RetrievalConfig configures a deterministic retrieval evaluator. K is a
// required positive cutoff. A nil Threshold selects [DefaultThreshold].
type RetrievalConfig struct {
	Metric    RetrievalMetric
	K         int
	Threshold *Score
}

func (c RetrievalConfig) validate() error {
	if err := c.Metric.Validate(); err != nil {
		return fmt.Errorf("%w: metric: %w", ErrInvalidConfig, err)
	}
	if c.K <= 0 {
		return fmt.Errorf("%w: K must be positive", ErrInvalidConfig)
	}
	return nil
}

func (c RetrievalConfig) threshold() (Score, error) {
	threshold, err := resolveThreshold(c.Threshold)
	if err != nil {
		return 0, fmt.Errorf("%w: threshold: %w", ErrInvalidConfig, err)
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

func (r relevanceSet) count(ranking []string) int {
	count := 0
	for _, identity := range ranking {
		if _, found := r[identity]; found {
			count++
		}
	}
	return count
}

func (r relevanceSet) precisionAt(ranking []string, k int) float64 {
	return float64(r.count(ranking)) / float64(k)
}

func (r relevanceSet) reciprocalRank(ranking []string) float64 {
	for index, identity := range ranking {
		if _, found := r[identity]; found {
			return 1 / float64(index+1)
		}
	}
	return 0
}

func (r relevanceSet) ndcgAt(ranking []string, k int) float64 {
	dcg := 0.0
	for index, identity := range ranking {
		if _, found := r[identity]; found {
			dcg += 1 / math.Log2(float64(index+2))
		}
	}
	idcg := 0.0
	for index := range min(k, len(r)) {
		idcg += 1 / math.Log2(float64(index+2))
	}
	return dcg / idcg
}

func (m RetrievalMetric) score(ranking []string, relevant relevanceSet, k int) float64 {
	switch m {
	case RetrievalPrecision:
		return relevant.precisionAt(ranking, k)
	case RetrievalRecall:
		return float64(relevant.count(ranking)) / float64(len(relevant))
	case RetrievalReciprocalRank:
		return relevant.reciprocalRank(ranking)
	case RetrievalNDCG:
		return relevant.ndcgAt(ranking, k)
	default:
		return 0
	}
}

// RetrievalEvaluator measures one standard ranking metric at a fixed cutoff.
type RetrievalEvaluator struct {
	metric       RetrievalMetric
	k            int
	threshold    Score
	reportMetric Metric
}

// NewRetrievalEvaluator constructs a deterministic ranking evaluator.
func NewRetrievalEvaluator(config RetrievalConfig) (*RetrievalEvaluator, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	threshold, err := config.threshold()
	if err != nil {
		return nil, err
	}
	reportMetric, err := config.Metric.reportMetric(config.K)
	if err != nil {
		return nil, fmt.Errorf("%w: report metric: %w", ErrInvalidConfig, err)
	}
	return &RetrievalEvaluator{
		metric:       config.Metric,
		k:            config.K,
		threshold:    threshold,
		reportMetric: reportMetric,
	}, nil
}

// Evaluate calculates the configured ranking score without a model call.
func (r *RetrievalEvaluator) Evaluate(ctx context.Context, sample RetrievalSample) (Report, error) {
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	if err := sample.Validate(); err != nil {
		return Report{}, err
	}

	relevant := newRelevanceSet(sample.Relevant)
	ranking := sample.Retrieved[:min(r.k, len(sample.Retrieved))]

	value := r.metric.score(ranking, relevant, r.k)
	score, err := NewScore(value)
	if err != nil {
		return Report{}, fmt.Errorf("evaluation: calculate %s: %w", r.reportMetric, err)
	}
	report := Report{Metric: r.reportMetric, Passed: score.Passes(r.threshold), Score: score}
	if err := report.Validate(); err != nil {
		return Report{}, err
	}
	return report, nil
}
