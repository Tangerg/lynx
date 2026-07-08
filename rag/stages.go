package rag

import (
	"context"

	"github.com/Tangerg/lynx/core/document"
)

// Transformer rewrites a query to be more retrieval-friendly — translation,
// compression, ambiguity resolution, vocabulary normalization.
type Transformer interface {
	// Transform returns the rewritten query.
	Transform(ctx context.Context, query *Query) (*Query, error)
}

// TransformerFunc adapts a function to [Transformer].
type TransformerFunc func(context.Context, *Query) (*Query, error)

// Transform calls f(ctx, query).
func (f TransformerFunc) Transform(ctx context.Context, query *Query) (*Query, error) {
	return f(ctx, query)
}

// Expander turns one query into many — useful for poorly formed inputs
// (alternative phrasings) or complex problems (decompose into sub-queries).
type Expander interface {
	// Expand returns one or more queries derived from the input.
	Expand(ctx context.Context, query *Query) ([]*Query, error)
}

// ExpanderFunc adapts a function to [Expander].
type ExpanderFunc func(context.Context, *Query) ([]*Query, error)

// Expand calls f(ctx, query).
func (f ExpanderFunc) Expand(ctx context.Context, query *Query) ([]*Query, error) {
	return f(ctx, query)
}

// Retriever pulls candidate documents from a knowledge source.
type Retriever interface {
	// Retrieve returns documents relevant to the query.
	Retrieve(ctx context.Context, query *Query) ([]*document.Document, error)
}

// RetrieverFunc adapts a function to [Retriever].
type RetrieverFunc func(context.Context, *Query) ([]*document.Document, error)

// Retrieve calls f(ctx, query).
func (f RetrieverFunc) Retrieve(ctx context.Context, query *Query) ([]*document.Document, error) {
	return f(ctx, query)
}

// Refiner narrows candidate documents down to what the LLM should see.
type Refiner interface {
	// Refine returns the trimmed/re-ranked document list.
	Refine(ctx context.Context, query *Query, documents []*document.Document) ([]*document.Document, error)
}

// RefinerFunc adapts a function to [Refiner].
type RefinerFunc func(context.Context, *Query, []*document.Document) ([]*document.Document, error)

// Refine calls f(ctx, query, documents).
func (f RefinerFunc) Refine(ctx context.Context, query *Query, documents []*document.Document) ([]*document.Document, error) {
	return f(ctx, query, documents)
}

// Augmenter folds retrieved documents into the query so the LLM has the right
// context to answer.
type Augmenter interface {
	// Augment returns a new query enriched with documents.
	Augment(ctx context.Context, query *Query, documents []*document.Document) (*Query, error)
}

// AugmenterFunc adapts a function to [Augmenter].
type AugmenterFunc func(context.Context, *Query, []*document.Document) (*Query, error)

// Augment calls f(ctx, query, documents).
func (f AugmenterFunc) Augment(ctx context.Context, query *Query, documents []*document.Document) (*Query, error) {
	return f(ctx, query, documents)
}
