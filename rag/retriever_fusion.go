package rag

import (
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

	return reciprocalRankFusion{rankConstant: config.RankConstant, retrievers: owned}, nil
}

type reciprocalRankFusion struct {
	rankConstant int
	retrievers   []Retriever
}

func (r reciprocalRankFusion) Retrieve(ctx context.Context, query Query) (candidates Candidates, err error) {
	if err := query.Validate(); err != nil {
		return nil, err
	}
	ctx, span := startStageSpan(ctx, "retrieve")
	defer func() {
		finishSpan(span, err, attribute.Int(attrDocCount, len(candidates)))
	}()

	rankings, err := parallelResults(ctx, "rag.ReciprocalRankFusion", r.retrievers, "retriever",
		func(ctx context.Context, _ int, retriever Retriever) (Candidates, error) {
			return Retrieve(ctx, retriever, query)
		})
	if err != nil {
		return nil, err
	}
	return r.fuse(ctx, rankings)
}

func (r reciprocalRankFusion) fuse(ctx context.Context, rankings []Candidates) (Candidates, error) {
	positions := make(map[string]int)
	fused := make(Candidates, 0)

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

			contribution := 1 / float64(r.rankConstant+index+1)
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

	return fused.ranked(), nil
}
