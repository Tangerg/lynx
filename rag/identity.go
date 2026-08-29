package rag

import "context"

func IdentityTransformer() Transformer {
	return TransformerFunc(func(ctx context.Context, query Query) (Query, error) {
		if err := ctx.Err(); err != nil {
			return Query{}, err
		}
		if err := query.Validate(); err != nil {
			return Query{}, err
		}
		return query, nil
	})
}

func IdentityExpander() Expander {
	return ExpanderFunc(func(ctx context.Context, query Query) ([]Query, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := query.Validate(); err != nil {
			return nil, err
		}
		return []Query{query}, nil
	})
}

func NopRetriever() Retriever {
	return RetrieverFunc(func(ctx context.Context, query Query) (Candidates, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := query.Validate(); err != nil {
			return nil, err
		}
		return nil, nil
	})
}

func IdentityRefiner() Refiner {
	return RefinerFunc(func(ctx context.Context, query Query, candidates Candidates) (Candidates, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := query.Validate(); err != nil {
			return nil, err
		}
		if err := candidates.Validate(); err != nil {
			return nil, err
		}
		return candidates.Clone(), nil
	})
}

func IdentityAugmenter() Augmenter {
	return AugmenterFunc(func(ctx context.Context, query Query, candidates Candidates) (Augmentation, error) {
		if err := ctx.Err(); err != nil {
			return Augmentation{}, err
		}
		if err := query.Validate(); err != nil {
			return Augmentation{}, err
		}
		if err := candidates.Validate(); err != nil {
			return Augmentation{}, err
		}
		return NewAugmentation(query.Text())
	})
}
