package rag

import (
	"context"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/pkg/sets"
)

var _ DocumentRefiner = (*DeduplicationDocumentRefiner)(nil)

// DeduplicationDocumentRefiner drops duplicate documents from the
// retrieval candidate list, keying on [document.Document.ID] and
// preserving first-occurrence order. Useful when multiple retrievers
// surface overlapping results.
//
// Example:
//
//	pipe, _ := rag.NewPipeline(rag.PipelineConfig{
//	    DocumentRetrievers: []rag.DocumentRetriever{r1, r2},
//	    DocumentRefiners: []rag.DocumentRefiner{
//	        rag.NewDeduplicationDocumentRefiner(),
//	        rag.NewRankDocumentRefiner(5),
//	    },
//	})
type DeduplicationDocumentRefiner struct{}

// NewDeduplicationDocumentRefiner returns a stateless refiner — the
// struct has no fields; sharing one across goroutines is fine.
func NewDeduplicationDocumentRefiner() *DeduplicationDocumentRefiner {
	return &DeduplicationDocumentRefiner{}
}

// Refine returns documents with duplicate IDs removed, keeping the
// first occurrence in input order. Honors ctx cancellation.
func (d *DeduplicationDocumentRefiner) Refine(ctx context.Context, _ *Query, documents []*document.Document) ([]*document.Document, error) {
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
