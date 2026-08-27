package rag

import "context"

// IdentityTransformer returns a [Transformer] that returns the input query.
func IdentityTransformer() Transformer {
	return TransformerFunc(func(_ context.Context, query Query) (Query, error) {
		return query, nil
	})
}

// IdentityExpander returns an [Expander] that returns only the input query.
func IdentityExpander() Expander {
	return ExpanderFunc(func(_ context.Context, query Query) ([]Query, error) {
		return []Query{query}, nil
	})
}

// NopRetriever returns a [Retriever] that always returns no documents.
func NopRetriever() Retriever {
	return RetrieverFunc(func(context.Context, Query) (Candidates, error) {
		return nil, nil
	})
}

// IdentityRefiner returns a [Refiner] that returns the input documents.
func IdentityRefiner() Refiner {
	return RefinerFunc(func(_ context.Context, _ Query, candidates Candidates) (Candidates, error) {
		return candidates, nil
	})
}

// IdentityAugmenter returns an [Augmenter] that uses the query text unchanged.
func IdentityAugmenter() Augmenter {
	return AugmenterFunc(func(_ context.Context, query Query, _ Candidates) (Augmentation, error) {
		if err := query.Validate(); err != nil {
			return Augmentation{}, err
		}
		return NewAugmentation(query.Text())
	})
}
