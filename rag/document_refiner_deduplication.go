package rag

import (
	"context"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/pkg/sets"
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
func (d deduper) Refine(ctx context.Context, _ *Query, documents []*document.Document) ([]*document.Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	seen := sets.NewHashSet[string]()
	out := make([]*document.Document, 0, len(documents))

	for _, doc := range documents {
		if seen.Contains(doc.ID) {
			continue
		}
		seen.Add(doc.ID)
		out = append(out, doc)
	}
	return out, nil
}
