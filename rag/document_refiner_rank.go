package rag

import (
	"cmp"
	"context"
	"slices"

	"github.com/Tangerg/lynx/core/document"
)

var _ Refiner = topKRefiner{}

// topKRefiner sorts documents by [document.Document.Score] descending and keeps
// the top K.
type topKRefiner struct {
	topK int
}

// TopK returns a [Refiner] that sorts documents by score descending and keeps
// at most topK entries. Non-positive topK is treated as 1.
func TopK(topK int) Refiner {
	if topK < 1 {
		topK = 1
	}
	return topKRefiner{topK: topK}
}

// Refine sorts documents by score (descending) and returns at most
// topK entries. The input slice is not mutated. Honors ctx cancellation.
func (r topKRefiner) Refine(ctx context.Context, _ *Query, documents []*document.Document) ([]*document.Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	sorted := slices.Clone(documents)
	slices.SortFunc(sorted, func(a, b *document.Document) int {
		return cmp.Compare(b.Score, a.Score) // descending
	})

	if len(sorted) > r.topK {
		sorted = sorted[:r.topK]
	}
	return sorted, nil
}
