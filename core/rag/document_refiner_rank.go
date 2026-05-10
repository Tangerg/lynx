package rag

import (
	"cmp"
	"context"
	"slices"

	"github.com/Tangerg/lynx/core/document"
)

var _ DocumentRefiner = (*RankDocumentRefiner)(nil)

// RankDocumentRefiner sorts documents by [document.Document.Score]
// descending and keeps the top-K. Use it after retrieval to focus on
// the strongest matches and bound the prompt budget.
type RankDocumentRefiner struct {
	topK int
}

// NewRankDocumentRefiner builds a [RankDocumentRefiner]. Non-positive
// topK falls back to 1 — every retrieval should yield at least one
// document, never an empty result purely due to a misconfigured cap.
func NewRankDocumentRefiner(topK int) *RankDocumentRefiner {
	if topK < 1 {
		topK = 1
	}
	return &RankDocumentRefiner{topK: topK}
}

// Refine sorts documents by score (descending) and returns at most
// topK entries. The input slice is not mutated. Honors ctx cancellation.
func (r *RankDocumentRefiner) Refine(ctx context.Context, _ *Query, documents []*document.Document) ([]*document.Document, error) {
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
