package rag

import (
	"context"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/pkg/sets"
)

var _ DocumentRefiner = (*DeduplicationRefiner)(nil)

// DeduplicationRefiner drops duplicate documents from the
// retrieval candidate list, keying on [document.Document.ID] and
// preserving first-occurrence order. Useful when multiple retrievers
// surface overlapping results.
//
// Example:
//
//	pipe, _ := rag.NewPipeline(&rag.PipelineConfig{
//	    DocumentRetrievers: []rag.DocumentRetriever{r1, r2},
//	    DocumentRefiners: []rag.DocumentRefiner{
//	        rag.NewDeduplicationRefiner(),
//	        rag.NewRankRefiner(5),
//	    },
//	})
type DeduplicationRefiner struct{}

// NewDeduplicationRefiner returns a stateless refiner — the
// struct has no fields; sharing one across goroutines is fine.
func NewDeduplicationRefiner() *DeduplicationRefiner {
	return &DeduplicationRefiner{}
}

// Refine returns documents with duplicate IDs removed, keeping the
// first occurrence in input order. Honors ctx cancellation.
func (d *DeduplicationRefiner) Refine(ctx context.Context, _ *Query, documents []*document.Document) ([]*document.Document, error) {
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
