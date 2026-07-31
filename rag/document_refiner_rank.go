package rag

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
)

var _ Refiner = topKRefiner{}

// topKRefiner sorts candidates by score descending and keeps the top K.
type topKRefiner struct {
	topK int
}

// TopK returns a [Refiner] that sorts documents by score descending and keeps
// at most topK entries. topK must be positive.
func TopK(topK int) (Refiner, error) {
	if topK < 1 {
		return nil, errors.New("rag: top K must be positive")
	}
	return topKRefiner{topK: topK}, nil
}

// Refine sorts documents by score (descending) and returns at most
// topK entries. The input slice is not mutated. Honors ctx cancellation.
func (r topKRefiner) Refine(ctx context.Context, _ *Query, documents []Candidate) ([]Candidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for index, candidate := range documents {
		if err := candidate.Validate(); err != nil {
			return nil, fmt.Errorf("rag: rank candidate %d: %w", index, err)
		}
	}

	sorted := slices.Clone(documents)
	slices.SortStableFunc(sorted, func(a, b Candidate) int {
		return cmp.Compare(b.Score, a.Score) // descending; stable keeps retrieval order on ties
	})

	if len(sorted) > r.topK {
		sorted = sorted[:r.topK]
	}
	return sorted, nil
}
