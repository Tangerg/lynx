package rag

import "context"

func IdentityTransformer() Transformer {
	return TransformerFunc(func(_ context.Context, query Query) (Query, error) {
		return query, nil
	})
}

func IdentityExpander() Expander {
	return ExpanderFunc(func(_ context.Context, query Query) ([]Query, error) {
		return []Query{query}, nil
	})
}

func NopRetriever() Retriever {
	return RetrieverFunc(func(context.Context, Query) (Candidates, error) {
		return nil, nil
	})
}

func IdentityRefiner() Refiner {
	return RefinerFunc(func(_ context.Context, _ Query, candidates Candidates) (Candidates, error) {
		return candidates, nil
	})
}

func IdentityAugmenter() Augmenter {
	return AugmenterFunc(func(_ context.Context, query Query, _ Candidates) (Augmentation, error) {
		if err := query.Validate(); err != nil {
			return Augmentation{}, err
		}
		return NewAugmentation(query.Text())
	})
}
