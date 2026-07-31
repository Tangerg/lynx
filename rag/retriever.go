package rag

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Retrieve calls r.Retrieve after checking that r is non-nil.
func Retrieve(ctx context.Context, r Retriever, query *Query) ([]Candidate, error) {
	if isNilCapability(r) {
		return nil, ErrNilRetriever
	}
	if err := query.Validate(); err != nil {
		return nil, err
	}
	return r.Retrieve(ctx, query)
}

// Parallel returns a [Retriever] that runs retrievers concurrently and unions
// their documents. If at least one retriever succeeds, failed retrievers are
// recorded on the current span and the successful documents are returned.
func Parallel(retrievers ...Retriever) Retriever {
	return RetrieverFunc(func(ctx context.Context, query *Query) ([]Candidate, error) {
		if err := query.Validate(); err != nil {
			return nil, err
		}
		if len(retrievers) == 0 {
			return nil, ErrNilRetriever
		}
		ctx, span := startStageSpan(ctx, "retrieve")
		var err error
		var docs []Candidate
		defer func() {
			finishSpan(span, err, attribute.Int(attrDocCount, len(docs)))
		}()
		docs, err = parallelCollect(ctx, "rag.Parallel", retrievers, "retriever",
			func(ctx context.Context, _ int, retriever Retriever) ([]Candidate, error) {
				if isNilCapability(retriever) {
					return nil, ErrNilRetriever
				}
				return retriever.Retrieve(ctx, query)
			})
		return docs, err
	})
}

// WithTransformers returns a [Retriever] that rewrites the query through
// transformers before calling next.
func WithTransformers(next Retriever, transformers ...Transformer) Retriever {
	return RetrieverFunc(func(ctx context.Context, query *Query) ([]Candidate, error) {
		if isNilCapability(next) {
			return nil, ErrNilRetriever
		}
		if err := query.Validate(); err != nil {
			return nil, err
		}
		current := query
		for i, transformer := range transformers {
			if isNilCapability(transformer) {
				continue
			}
			var err error
			current, err = transformer.Transform(ctx, current)
			if err != nil {
				return nil, fmt.Errorf("rag: transformer %d: %w", i, err)
			}
			if err := current.Validate(); err != nil {
				return nil, err
			}
		}
		return next.Retrieve(ctx, current)
	})
}

// WithExpander returns a [Retriever] that expands one query into many and
// calls next for each expanded query in parallel.
func WithExpander(next Retriever, expander Expander) Retriever {
	return RetrieverFunc(func(ctx context.Context, query *Query) ([]Candidate, error) {
		if isNilCapability(next) {
			return nil, ErrNilRetriever
		}
		if err := query.Validate(); err != nil {
			return nil, err
		}
		if isNilCapability(expander) {
			return next.Retrieve(ctx, query)
		}
		queries, err := expander.Expand(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("rag: expand query: %w", err)
		}
		if len(queries) == 0 {
			queries = []*Query{query}
		}
		return parallelCollect(ctx, "rag.WithExpander", queries, "query",
			func(ctx context.Context, _ int, q *Query) ([]Candidate, error) {
				if err := q.Validate(); err != nil {
					return nil, err
				}
				return next.Retrieve(ctx, q)
			})
	})
}

// WithRefiners returns a [Retriever] that calls next and then applies
// refiners to the returned documents in order.
func WithRefiners(next Retriever, refiners ...Refiner) Retriever {
	return RetrieverFunc(func(ctx context.Context, query *Query) ([]Candidate, error) {
		if isNilCapability(next) {
			return nil, ErrNilRetriever
		}
		if err := query.Validate(); err != nil {
			return nil, err
		}
		docs, err := next.Retrieve(ctx, query)
		if err != nil {
			return nil, err
		}
		for i, refiner := range refiners {
			if isNilCapability(refiner) {
				continue
			}
			docs, err = refiner.Refine(ctx, query, docs)
			if err != nil {
				return nil, fmt.Errorf("rag: refiner %d: %w", i, err)
			}
		}
		return docs, nil
	})
}

func parallelCollect[Item, Out any](
	ctx context.Context,
	op string,
	items []Item,
	itemLabel string,
	fn func(context.Context, int, Item) ([]Out, error),
) ([]Out, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Each goroutine writes only its own index, so no lock is needed and the
	// union stays in input order regardless of completion order — Dedup's
	// first-occurrence representative and TopK's tie-break depend on it.
	results := make([][]Out, len(items))
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

	var out []Out
	for _, block := range results {
		out = append(out, block...)
	}
	errs := make([]error, 0, len(failures))
	for _, err := range failures {
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) == 0 {
		return out, nil
	}
	if len(errs) == len(items) {
		return nil, fmt.Errorf("%s: every %s failed: %w", op, itemLabel, errors.Join(errs...))
	}
	span := trace.SpanFromContext(ctx)
	for _, err := range errs {
		span.RecordError(err)
	}
	return out, nil
}
