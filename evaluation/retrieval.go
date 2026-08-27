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

// RetrievalEvaluator measures one standard ranking metric at a fixed cutoff.
type RetrievalEvaluator struct {
	metric       RetrievalMetric
	k            int
	threshold    Score
	reportMetric Metric
}

// NewRetrievalEvaluator constructs a deterministic ranking evaluator.
func NewRetrievalEvaluator(config RetrievalConfig) (*RetrievalEvaluator, error) {
	if err := config.Metric.validate(); err != nil {
		return nil, fmt.Errorf("%w: metric: %w", ErrInvalidConfig, err)
	}
	if config.K <= 0 {
		return nil, fmt.Errorf("%w: K must be positive", ErrInvalidConfig)
	}
	threshold := DefaultThreshold
	if config.Threshold != nil {
		threshold = *config.Threshold
	}
	if err := threshold.Validate(); err != nil {
		return nil, fmt.Errorf("%w: threshold: %w", ErrInvalidConfig, err)
	}
	reportMetric, err := NewMetric(fmt.Sprintf("retrieval/%s@%d", config.Metric, config.K))
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

func (m RetrievalMetric) validate() error {
	switch m {
	case RetrievalPrecision, RetrievalRecall, RetrievalReciprocalRank, RetrievalNDCG:
		return nil
	default:
		return fmt.Errorf("unsupported retrieval metric %q", m)
	}
}

// Evaluate calculates the configured ranking score without a model call.
func (r *RetrievalEvaluator) Evaluate(ctx context.Context, sample RetrievalSample) (Report, error) {
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	if err := sample.Validate(); err != nil {
		return Report{}, err
	}

	relevant := make(map[string]struct{}, len(sample.Relevant))
	for _, id := range sample.Relevant {
		relevant[id] = struct{}{}
	}
	ranking := sample.Retrieved[:min(r.k, len(sample.Retrieved))]

	var value float64
	switch r.metric {
	case RetrievalPrecision:
		value = precisionAt(ranking, relevant, r.k)
	case RetrievalRecall:
		value = float64(relevantCount(ranking, relevant)) / float64(len(relevant))
	case RetrievalReciprocalRank:
		value = reciprocalRank(ranking, relevant)
	case RetrievalNDCG:
		value = ndcgAt(ranking, relevant, r.k)
	}
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

func relevantCount(ranking []string, relevant map[string]struct{}) int {
	count := 0
	for _, id := range ranking {
		if _, found := relevant[id]; found {
			count++
		}
	}
	return count
}

func precisionAt(ranking []string, relevant map[string]struct{}, k int) float64 {
	return float64(relevantCount(ranking, relevant)) / float64(k)
}

func reciprocalRank(ranking []string, relevant map[string]struct{}) float64 {
	for index, id := range ranking {
		if _, found := relevant[id]; found {
			return 1 / float64(index+1)
		}
	}
	return 0
}

func ndcgAt(ranking []string, relevant map[string]struct{}, k int) float64 {
	dcg := 0.0
	for index, id := range ranking {
		if _, found := relevant[id]; found {
			dcg += 1 / math.Log2(float64(index+2))
		}
	}
	idcg := 0.0
	for index := range min(k, len(relevant)) {
		idcg += 1 / math.Log2(float64(index+2))
	}
	return dcg / idcg
}
