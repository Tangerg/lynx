package rag

import (
	"context"

	"github.com/Tangerg/lynx/ai/media/document"
)

// QueryExpander expands the input query into a list of queries, addressing challenges
// such as poorly formed queries by providing alternative query formulations, or by
// breaking down complex problems into simpler sub-queries.
type QueryExpander interface {
	// Expand expands the given query into a list of queries.
	// It returns an error if the expansion process fails.
	Expand(ctx context.Context, query *Query) ([]*Query, error)
}

// QueryTransformer transforms the input query to make it more effective for retrieval
// tasks, addressing challenges such as poorly formed queries, ambiguous terms, complex
// vocabulary, or unsupported languages.
type QueryTransformer interface {
	// Transform transforms the given query according to the implemented strategy.
	// It returns the transformed query or an error if the transformation fails.
	Transform(ctx context.Context, query *Query) (*Query, error)
}

// QueryAugmenter augments an input query with additional data, useful to provide a
// large language model with the necessary context to answer the user query.
type QueryAugmenter interface {
	// Augment augments the user query with contextual data from the provided documents.
	// It returns the augmented query or an error if the augmentation process fails.
	Augment(ctx context.Context, query *Query, documents []*document.Document) (*Query, error)
}

// DocumentRetriever retrieves documents from an underlying data source,
// such as a search engine, a vector store, a database, or a knowledge graph.
type DocumentRetriever interface {
	// Retrieve retrieves relevant documents from an underlying data source based on
	// the given query. It returns the list of relevant documents or an error if the
	// retrieval process fails.
	Retrieve(ctx context.Context, query *Query) ([]*document.Document, error)
}

// DocumentRefiner refines retrieved documents based on a query, addressing
// challenges such as "lost-in-the-middle", context length restrictions from the model,
// and the need to reduce noise and redundancy in the retrieved information.
//
// For example, it could rank documents based on their relevance to the query, remove
// irrelevant or redundant documents, or compress the content of each document to reduce
// noise and redundancy.
type DocumentRefiner interface {
	// Refine refines the list of documents based on the query.
	// It returns the refined list of documents or an error if the refinement process fails.
	Refine(ctx context.Context, query *Query, documents []*document.Document) ([]*document.Document, error)
}
