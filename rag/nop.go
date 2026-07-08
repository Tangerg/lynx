package rag

import (
	"context"

	"github.com/Tangerg/lynx/core/document"
)

// IdentityTransformer returns a [Transformer] that returns the input query.
func IdentityTransformer() Transformer {
	return TransformerFunc(func(_ context.Context, query *Query) (*Query, error) {
		return query, nil
	})
}

// IdentityExpander returns an [Expander] that returns only the input query.
func IdentityExpander() Expander {
	return ExpanderFunc(func(_ context.Context, query *Query) ([]*Query, error) {
		return []*Query{query}, nil
	})
}

// NoopRetriever returns a [Retriever] that always returns no documents.
func NoopRetriever() Retriever {
	return RetrieverFunc(func(context.Context, *Query) ([]*document.Document, error) {
		return nil, nil
	})
}

// IdentityRefiner returns a [Refiner] that returns the input documents.
func IdentityRefiner() Refiner {
	return RefinerFunc(func(_ context.Context, _ *Query, docs []*document.Document) ([]*document.Document, error) {
		return docs, nil
	})
}

// IdentityAugmenter returns an [Augmenter] that returns the input query.
func IdentityAugmenter() Augmenter {
	return AugmenterFunc(func(_ context.Context, query *Query, _ []*document.Document) (*Query, error) {
		return query, nil
	})
}
