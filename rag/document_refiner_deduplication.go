package rag

import (
	"context"
	"fmt"
)

var _ Refiner = deduper{}

// deduper drops duplicate documents by [document.Document.ID], preserving
// first-occurrence order.
type deduper struct{}

// Dedup returns a [Refiner] that drops duplicate documents by
// [document.Document.ID], preserving first-occurrence order.
func Dedup() Refiner {
	return deduper{}
}

// Refine returns documents with duplicate IDs removed, keeping the
// first occurrence in input order. Honors ctx cancellation.
func (d deduper) Refine(ctx context.Context, _ *Query, documents []Candidate) ([]Candidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	seen := make(map[string]struct{}, len(documents))
	out := make([]Candidate, 0, len(documents))

	for index, candidate := range documents {
		if err := candidate.Validate(); err != nil {
			return nil, fmt.Errorf("rag: deduplicate candidate %d: %w", index, err)
		}
		id := candidate.Document.ID
		if id == "" {
			out = append(out, candidate)
			continue
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, candidate)
	}
	return out, nil
}
