package refiners

import (
	"context"

	"github.com/Tangerg/lynx/ai/media/document"
	"github.com/Tangerg/lynx/ai/rag"
	"github.com/Tangerg/lynx/pkg/sets"
)

var _ rag.DocumentRefiner = (*DeduplicationRefiner)(nil)

// DeduplicationRefiner removes duplicate documents from the retrieved results based
// on document IDs, preserving the order of first occurrence.
//
// This refiner is useful for:
//   - Eliminating redundant documents when the same content is retrieved multiple times
//   - Reducing processing overhead by removing duplicate entries
//   - Improving result quality by avoiding repetitive information
//   - Maintaining deterministic results by preserving first-occurrence order
type DeduplicationRefiner struct{}

func NewDeduplicationRefiner() *DeduplicationRefiner {
	return &DeduplicationRefiner{}
}

func (d *DeduplicationRefiner) Refine(_ context.Context, _ *rag.Query, documents []*document.Document) ([]*document.Document, error) {
	seenIDs := sets.NewHashSet[string]()
	uniqueDocuments := make([]*document.Document, 0, len(documents))

	for _, doc := range documents {
		if seenIDs.Contains(doc.ID) {
			continue
		}

		seenIDs.Add(doc.ID)
		uniqueDocuments = append(uniqueDocuments, doc)
	}

	return uniqueDocuments, nil
}
