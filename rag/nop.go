package rag

import (
	"context"
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

// NopRetriever returns a [Retriever] that always returns no documents.
func NopRetriever() Retriever {
	return RetrieverFunc(func(context.Context, *Query) ([]Candidate, error) {
		return nil, nil
	})
}

// IdentityRefiner returns a [Refiner] that returns the input documents.
func IdentityRefiner() Refiner {
	return RefinerFunc(func(_ context.Context, _ *Query, docs []Candidate) ([]Candidate, error) {
		return docs, nil
	})
}

// IdentityAugmenter returns an [Augmenter] that returns the input query.
func IdentityAugmenter() Augmenter {
	return AugmenterFunc(func(_ context.Context, query *Query, _ []Candidate) (*Query, error) {
		return query, nil
	})
}
