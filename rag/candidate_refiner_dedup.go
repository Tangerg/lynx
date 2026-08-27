package rag

import (
	"context"
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
func (d deduper) Refine(ctx context.Context, _ Query, candidates Candidates) (Candidates, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if err := candidates.Validate(); err != nil {
		return nil, err
	}
	return candidates.uniqueBest(), nil
}
