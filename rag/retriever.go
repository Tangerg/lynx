package rag

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/samber/lo"
	"go.opentelemetry.io/otel/attribute"
)

// Retrieve is the validation boundary around a retriever, including custom
// implementations supplied by callers.
func Retrieve(ctx context.Context, retriever Retriever, query Query) (Candidates, error) {
	if lo.IsNil(retriever) {
		return nil, ErrNilRetriever
	}
	if err := query.Validate(); err != nil {
		return nil, err
	}
	candidates, err := retriever.Retrieve(ctx, query)
	if err != nil {
		return nil, err
	}
	if err := candidates.Validate(); err != nil {
		return nil, err
	}
	return candidates, nil
}

// Parallel returns a [Retriever] that runs retrievers concurrently and unions
// their documents in declaration order. Every retriever must succeed; partial
// retrieval is not silently presented as a complete result.
func Parallel(retrievers ...Retriever) (Retriever, error) {
	if len(retrievers) == 0 {
		return nil, ErrNilRetriever
	}
	owned := slices.Clone(retrievers)
	for index, retriever := range owned {
		if lo.IsNil(retriever) {
			return nil, fmt.Errorf("rag.Parallel: retriever %d: %w", index, ErrNilRetriever)
		}
	}

	return RetrieverFunc(func(ctx context.Context, query Query) (Candidates, error) {
		if err := query.Validate(); err != nil {
			return nil, err
		}
		ctx, span := startStageSpan(ctx, retrieveStage)
		var err error
		var docs Candidates
		defer func() {
			finishSpan(span, err, attribute.Int(attrDocCount, len(docs)))
		}()
		docs, err = parallelCandidates(ctx, "rag.Parallel", owned, "retriever",
			func(ctx context.Context, _ int, retriever Retriever) (Candidates, error) {
				return Retrieve(ctx, retriever, query)
			})
		return docs, err
	}), nil
}

// WithTransformers returns a [Retriever] that rewrites the query through
// transformers before calling next.
func WithTransformers(next Retriever, transformers ...Transformer) (Retriever, error) {
	if lo.IsNil(next) {
		return nil, ErrNilRetriever
	}
	owned := slices.Clone(transformers)
	for index, transformer := range owned {
		if lo.IsNil(transformer) {
			return nil, fmt.Errorf("rag.WithTransformers: transformer %d: %w", index, ErrNilTransformer)
		}
	}

	return RetrieverFunc(func(ctx context.Context, query Query) (Candidates, error) {
		if err := query.Validate(); err != nil {
			return nil, err
		}
		current := query
		for i, transformer := range owned {
			var err error
			current, err = transformer.Transform(ctx, current)
			if err != nil {
				return nil, fmt.Errorf("rag: transformer %d: %w", i, err)
			}
			if err := current.Validate(); err != nil {
				return nil, fmt.Errorf("rag: transformer %d returned an invalid query: %w", i, err)
			}
		}
		return Retrieve(ctx, next, current)
	}), nil
}

// WithExpander returns a [Retriever] that expands one query into many and
// calls next for each expanded query in parallel.
func WithExpander(next Retriever, expander Expander) (Retriever, error) {
	if lo.IsNil(next) {
		return nil, ErrNilRetriever
	}
	if lo.IsNil(expander) {
		return nil, ErrNilExpander
	}

	return RetrieverFunc(func(ctx context.Context, query Query) (Candidates, error) {
		if err := query.Validate(); err != nil {
			return nil, err
		}
		queries, err := expander.Expand(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("rag: expand query: %w", err)
		}
		if len(queries) == 0 {
			return nil, ErrEmptyExpansion
		}
		queries = slices.Clone(queries)
		return parallelCandidates(ctx, "rag.WithExpander", queries, "query",
			func(ctx context.Context, _ int, q Query) (Candidates, error) {
				return Retrieve(ctx, next, q)
			})
	}), nil
}

// WithRefiners returns a [Retriever] that calls next and then applies
// refiners to the returned documents in order.
func WithRefiners(next Retriever, refiners ...Refiner) (Retriever, error) {
	if lo.IsNil(next) {
		return nil, ErrNilRetriever
	}
	owned := slices.Clone(refiners)
	for index, refiner := range owned {
		if lo.IsNil(refiner) {
			return nil, fmt.Errorf("rag.WithRefiners: refiner %d: %w", index, ErrNilRefiner)
		}
	}

	return RetrieverFunc(func(ctx context.Context, query Query) (Candidates, error) {
		if err := query.Validate(); err != nil {
			return nil, err
		}
		docs, err := Retrieve(ctx, next, query)
		if err != nil {
			return nil, err
		}
		for i, refiner := range owned {
			docs, err = refiner.Refine(ctx, query, docs)
			if err != nil {
				return nil, fmt.Errorf("rag: refiner %d: %w", i, err)
			}
			if err := docs.Validate(); err != nil {
				return nil, fmt.Errorf("rag: refiner %d returned invalid candidates: %w", i, err)
			}
		}
		return docs, nil
	}), nil
}

func parallelCandidates[Item any](
	ctx context.Context,
	op string,
	items []Item,
	itemLabel string,
	fn func(context.Context, int, Item) (Candidates, error),
) (Candidates, error) {
	results, err := parallelResults(ctx, op, items, itemLabel, fn)
	if err != nil {
		return nil, err
	}
	var out Candidates
	for _, block := range results {
		out = append(out, block...)
	}
	return out, nil
}

func parallelResults[Item, Out any](
	ctx context.Context,
	op string,
	items []Item,
	itemLabel string,
	fn func(context.Context, int, Item) (Out, error),
) ([]Out, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Each goroutine writes only its own index, so no lock is needed and the
	// union stays in input order regardless of completion order — Dedup's
	// identity order and equal-score selection, plus TopK's tie-break, depend on
	// it.
	results := make([]Out, len(items))
	failures := make([]error, len(items))

	var wg sync.WaitGroup
	for index, item := range items {
		wg.Go(func() {
			result, err := fn(ctx, index, item)
			if err != nil {
				failures[index] = fmt.Errorf("%s #%d: %w", itemLabel, index, err)
				return
			}
			results[index] = result
		})
	}
	wg.Wait()

	errs := make([]error, 0, len(failures))
	for _, err := range failures {
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) != 0 {
		return nil, fmt.Errorf("%s: %w", op, errors.Join(errs...))
	}

	return results, nil
}
