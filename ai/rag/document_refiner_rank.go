package rag

import (
	"context"
	"sort"

	"github.com/Tangerg/lynx/ai/media/document"
)

var _ DocumentRefiner = (*RankDocumentRefiner)(nil)

// RankDocumentRefiner sorts documents by their relevance scores in descending order and
// returns only the top-K highest scoring documents.
//
// This refiner is useful for:
//   - Limiting the number of documents passed to downstream components
//   - Focusing on the most relevant results based on similarity scores
//   - Reducing token usage by filtering out lower-quality matches
//   - Improving response quality by using only the best matches
type RankDocumentRefiner struct {
	topK int
}

func NewRankDocumentRefiner(topK int) *RankDocumentRefiner {
	if topK < 1 {
		topK = 1
	}

	return &RankDocumentRefiner{topK: topK}
}

func (r *RankDocumentRefiner) Refine(ctx context.Context, _ *Query, documents []*document.Document) ([]*document.Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

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
