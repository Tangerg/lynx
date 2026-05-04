// Package rag implements the standard Retrieval-Augmented Generation
// pipeline: take a user query, transform it, expand it, retrieve
// relevant documents, refine them, and augment the original query with
// the retrieved context. The interfaces in this file are the building
// blocks; concrete implementations live alongside (query_*.go,
// document_*.go) and the [Pipeline] glue is in pipeline.go.
package rag

import (
	"context"

	"github.com/Tangerg/lynx/core/document"
)

// QueryExpander turns one query into many — useful for poorly formed
// inputs (alternative phrasings) or complex problems (decompose into
// sub-queries the retriever can answer in parallel).
type QueryExpander interface {
	// Expand returns one or more queries derived from the input.
	Expand(ctx context.Context, query *Query) ([]*Query, error)
}

// QueryTransformer rewrites a query to be more retrieval-friendly —
// translation, compression, ambiguity resolution, vocabulary
// normalization. Transformations chain in [Pipeline].
type QueryTransformer interface {
	// Transform returns the rewritten query.
	Transform(ctx context.Context, query *Query) (*Query, error)
}

// QueryAugmenter folds retrieved documents into the query so the LLM
// has the right context to answer.
type QueryAugmenter interface {
	// Augment returns a new query enriched with documents.
	Augment(ctx context.Context, query *Query, documents []*document.Document) (*Query, error)
}

// DocumentRetriever pulls candidate documents from a knowledge source
// (vector store, search engine, database, knowledge graph).
type DocumentRetriever interface {
	// Retrieve returns documents relevant to the query.
	Retrieve(ctx context.Context, query *Query) ([]*document.Document, error)
}

// DocumentRefiner narrows a candidate document list down to what the
// LLM should actually see — re-rank by relevance, drop near-duplicates,
// trim to fit the prompt budget. Addresses the "lost in the middle"
// problem and reduces noise before the LLM sees the context.
type DocumentRefiner interface {
	// Refine returns the trimmed/re-ranked document list.
	Refine(ctx context.Context, query *Query, documents []*document.Document) ([]*document.Document, error)
}
