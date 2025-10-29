package refiners

import (
	"context"
	"sort"

	"github.com/Tangerg/lynx/ai/media/document"
	"github.com/Tangerg/lynx/ai/rag"
)

var _ rag.DocumentRefiner = (*RankRefiner)(nil)

// RankRefiner sorts documents by their relevance scores in descending order and
// returns only the top-K highest scoring documents.
//
// This refiner is useful for:
//   - Limiting the number of documents passed to downstream components
//   - Focusing on the most relevant results based on similarity scores
//   - Reducing token usage by filtering out lower-quality matches
//   - Improving response quality by using only the best matches
type RankRefiner struct {
	topK int
}

func NewRankRefiner(topK int) *RankRefiner {
	if topK < 1 {
		topK = 1
	}

	return &RankRefiner{topK: topK}
}

func (r *RankRefiner) Refine(_ context.Context, _ *rag.Query, documents []*document.Document) ([]*document.Document, error) {
	sortedDocuments := make([]*document.Document, len(documents))
	copy(sortedDocuments, documents)

	sort.Slice(sortedDocuments, func(i, j int) bool {
		return sortedDocuments[i].Score > sortedDocuments[j].Score
	})

	if len(sortedDocuments) > r.topK {
		sortedDocuments = sortedDocuments[:r.topK]
	}

	return sortedDocuments, nil
}
