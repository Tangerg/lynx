package rag

import (
	"context"
	"fmt"
)

var _ Refiner = deduper{}

// deduper collapses retrieval candidates that identify the same document.
type deduper struct{}

// Dedup returns a [Refiner] that keeps the highest-scoring candidate for each
// non-empty [document.Document.ID]. Document identities retain first-seen
// order, and equal scores retain the first candidate. Documents without an ID
// remain distinct because the framework cannot prove they are duplicates.
func Dedup() Refiner {
	return deduper{}
}

// Refine returns the best candidate for every known document identity. Honors
// ctx cancellation.
func (d deduper) Refine(ctx context.Context, _ *Query, documents []Candidate) ([]Candidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	for index, candidate := range documents {
		if err := candidate.Validate(); err != nil {
			return nil, fmt.Errorf("rag: deduplicate candidate %d: %w", index, err)
		}
	}
	return uniqueBestCandidates(documents), nil
}

func uniqueBestCandidates(candidates []Candidate) []Candidate {
	positions := make(map[string]int, len(candidates))
	unique := make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		id := candidate.Document.ID
		if id == "" {
			unique = append(unique, candidate)
			continue
		}
		position, exists := positions[id]
		if !exists {
			positions[id] = len(unique)
			unique = append(unique, candidate)
			continue
		}
		if candidate.Score > unique[position].Score {
			unique[position] = candidate
		}
	}
	return unique
}
