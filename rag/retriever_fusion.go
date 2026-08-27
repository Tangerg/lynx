package rag

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/samber/lo"
	"go.opentelemetry.io/otel/attribute"
)

// ErrInvalidRankConstant reports an invalid reciprocal-rank denominator
// constant.
var ErrInvalidRankConstant = errors.New("rag: reciprocal-rank constant must be positive")

// DefaultReciprocalRankConstant is the conventional RRF smoothing constant.
const DefaultReciprocalRankConstant = 60

// ReciprocalRankFusionConfig configures [ReciprocalRankFusion]. RankConstant
// is the positive constant added to each one-based rank.
type ReciprocalRankFusionConfig struct {
	RankConstant int
}

// Validate checks whether the fusion configuration is usable.
func (c ReciprocalRankFusionConfig) Validate() error {
	if c.RankConstant <= 0 {
		return ErrInvalidRankConstant
	}
	return nil
}

// ReciprocalRankFusion returns a retriever that concurrently executes each
// input retriever and fuses their ordered results using reciprocal-rank
// fusion. Raw candidate scores are deliberately ignored because independent
// retrievers commonly use incomparable score scales.
func ReciprocalRankFusion(config ReciprocalRankFusionConfig, retrievers ...Retriever) (Retriever, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if len(retrievers) == 0 {
		return nil, ErrNilRetriever
	}
	owned := slices.Clone(retrievers)
	for index, retriever := range owned {
		if lo.IsNil(retriever) {
			return nil, fmt.Errorf("rag.ReciprocalRankFusion: retriever %d: %w", index, ErrNilRetriever)
		}
	}

	return RetrieverFunc(func(ctx context.Context, query *Query) ([]Candidate, error) {
		if err := query.Validate(); err != nil {
			return nil, err
		}
		ctx, span := startStageSpan(ctx, "retrieve")
		var err error
		var candidates []Candidate
		defer func() {
			finishSpan(span, err, attribute.Int(attrDocCount, len(candidates)))
		}()

		rankings, err := parallelResults(ctx, "rag.ReciprocalRankFusion", owned, "retriever",
			func(ctx context.Context, _ int, retriever Retriever) ([]Candidate, error) {
				return Retrieve(ctx, retriever, query)
			})
		if err != nil {
			return nil, err
		}
		candidates, err = fuseRankings(ctx, config.RankConstant, rankings)
		return candidates, err
	}), nil
}

func fuseRankings(ctx context.Context, rankConstant int, rankings [][]Candidate) ([]Candidate, error) {
	positions := make(map[string]int)
	fused := make([]Candidate, 0)

	for _, ranking := range rankings {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		seen := make(map[string]struct{}, len(ranking))
		for index, candidate := range ranking {
			identity := candidate.Document.ID
			if identity != "" {
				if _, duplicate := seen[identity]; duplicate {
					continue
				}
				seen[identity] = struct{}{}
			}

			contribution := 1 / float64(rankConstant+index+1)
			if identity == "" {
				candidate.Score = contribution
				fused = append(fused, candidate)
				continue
			}
			position, found := positions[identity]
			if found {
				fused[position].Score += contribution
				continue
			}
			candidate.Score = contribution
			positions[identity] = len(fused)
			fused = append(fused, candidate)
		}
	}

	slices.SortStableFunc(fused, func(left, right Candidate) int {
		return cmp.Compare(right.Score, left.Score)
	})
	return fused, nil
}
