package rag

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/Tangerg/lynx/core/document"
)

// PipelineConfig wires the components that make up a [Pipeline].
// At least one [DocumentRetriever] is required; every other slot
// defaults to [Nop] so callers only fill what they need.
type PipelineConfig struct {
	// QueryTransformers chain in the order given. Each receives the
	// previous output. Optional.
	QueryTransformers []QueryTransformer

	// QueryExpander runs after transformations to fan out into
	// multiple queries. Defaults to [Nop] (single-query passthrough).
	QueryExpander QueryExpander

	// DocumentRetrievers run in parallel; their results are unioned.
	// Required — at least one entry.
	DocumentRetrievers []DocumentRetriever

	// DocumentRefiners chain after retrieval to re-rank, dedupe, or
	// trim the candidate list. Optional.
	DocumentRefiners []DocumentRefiner

	// QueryAugmenter folds the refined documents into the final query.
	// Defaults to [Nop] (no augmentation).
	QueryAugmenter QueryAugmenter
}

// validate fills in defaults and rejects configurations missing the
// required pieces.
func (c *PipelineConfig) validate() error {
	if len(c.DocumentRetrievers) == 0 {
		return errors.New("rag.PipelineConfig: at least one DocumentRetriever is required")
	}

	if c.QueryExpander == nil {
		c.QueryExpander = NewNop()
	}
	if c.QueryAugmenter == nil {
		c.QueryAugmenter = NewNop()
	}
	return nil
}

// Pipeline runs a query through the full RAG flow: transform → expand
// → retrieve → refine → augment.
//
// Example:
//
//	pipe, err := rag.NewPipeline(rag.PipelineConfig{
//	    DocumentRetrievers: []rag.DocumentRetriever{retriever},
//	    QueryAugmenter:     contextual,
//	})
//	augmented, docs, err := pipe.Run(ctx, "what is GOAP?")
type Pipeline struct {
	queryTransformers  []QueryTransformer
	queryExpander      QueryExpander
	documentRetrievers []DocumentRetriever
	documentRefiners   []DocumentRefiner
	queryAugmenter     QueryAugmenter
}

// NewPipeline builds a [Pipeline] from config. Returns an error when
// the configuration fails validation.
func NewPipeline(config PipelineConfig) (*Pipeline, error) {
	if err := config.validate(); err != nil {
		return nil, fmt.Errorf("rag.NewPipeline: %w", err)
	}

	return &Pipeline{
		queryTransformers:  config.QueryTransformers,
		queryExpander:      config.QueryExpander,
		documentRetrievers: config.DocumentRetrievers,
		documentRefiners:   config.DocumentRefiners,
		queryAugmenter:     config.QueryAugmenter,
	}, nil
}

// Execute runs every stage and returns the final augmented query
// together with the refined document list. An error from any stage
// short-circuits the pipeline.
func (p *Pipeline) Execute(ctx context.Context, query *Query) (*Query, []*document.Document, error) {
	transformed, err := p.transformQuery(ctx, query)
	if err != nil {
		return nil, nil, fmt.Errorf("rag.Pipeline: transform stage: %w", err)
	}

	expanded, err := p.expandQuery(ctx, transformed)
	if err != nil {
		return nil, nil, fmt.Errorf("rag.Pipeline: expand stage: %w", err)
	}

	retrieved, err := p.retrieveByQueries(ctx, expanded)
	if err != nil {
		return nil, nil, fmt.Errorf("rag.Pipeline: retrieve stage: %w", err)
	}

	refined, err := p.refineDocuments(ctx, query, retrieved)
	if err != nil {
		return nil, nil, fmt.Errorf("rag.Pipeline: refine stage: %w", err)
	}

	augmented, err := p.augmentQuery(ctx, query, refined)
	if err != nil {
		return nil, nil, fmt.Errorf("rag.Pipeline: augment stage: %w", err)
	}
	return augmented, refined, nil
}

// Run is a convenience wrapper that constructs a [Query] from text and
// invokes [Pipeline.Execute].
func (p *Pipeline) Run(ctx context.Context, text string) (*Query, []*document.Document, error) {
	query, err := NewQuery(text)
	if err != nil {
		return nil, nil, fmt.Errorf("rag.Pipeline.Run: %w", err)
	}
	return p.Execute(ctx, query)
}

// transformQuery applies each registered [QueryTransformer] in order.
func (p *Pipeline) transformQuery(ctx context.Context, query *Query) (*Query, error) {
	current := query
	for i, transformer := range p.queryTransformers {
		next, err := transformer.Transform(ctx, current)
		if err != nil {
			return nil, fmt.Errorf("transformer #%d: %w", i, err)
		}
		current = next
	}
	return current, nil
}

// expandQuery fans the query out to one-or-more queries via the
// configured [QueryExpander].
func (p *Pipeline) expandQuery(ctx context.Context, query *Query) ([]*Query, error) {
	return p.queryExpander.Expand(ctx, query)
}

// retrieveByQuery runs every retriever in parallel and unions the
// results. A partial failure (some retrievers fail, others return
// docs) returns the docs we have rather than failing the whole
// retrieval.
func (p *Pipeline) retrieveByQuery(ctx context.Context, query *Query) ([]*document.Document, error) {
	var (
		mu   sync.Mutex
		docs []*document.Document
	)

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(len(p.documentRetrievers))

	for index, retriever := range p.documentRetrievers {
		g.Go(func() error {
			retrieved, err := retriever.Retrieve(gctx, query)
			if err != nil {
				return fmt.Errorf("retriever #%d: %w", index, err)
			}
			mu.Lock()
			docs = append(docs, retrieved...)
			mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		if len(docs) == 0 {
			return nil, fmt.Errorf("all retrievers failed: %w", err)
		}
		return docs, nil
	}
	return docs, nil
}

// retrieveByQueries runs the per-query retrieval fan-in for every
// expanded query in parallel.
func (p *Pipeline) retrieveByQueries(ctx context.Context, queries []*Query) ([]*document.Document, error) {
	var (
		mu   sync.Mutex
		docs []*document.Document
	)

	g, _ := errgroup.WithContext(ctx)
	g.SetLimit(len(queries))

	for index, query := range queries {
		g.Go(func() error {
			retrieved, err := p.retrieveByQuery(ctx, query)
			if err != nil {
				return fmt.Errorf("query #%d: %w", index, err)
			}
			mu.Lock()
			docs = append(docs, retrieved...)
			mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		if len(docs) == 0 {
			return nil, fmt.Errorf("all query retrievals failed: %w", err)
		}
		return docs, nil
	}
	return docs, nil
}

// refineDocuments applies each registered [DocumentRefiner] in order.
func (p *Pipeline) refineDocuments(ctx context.Context, query *Query, docs []*document.Document) ([]*document.Document, error) {
	current := docs
	for i, refiner := range p.documentRefiners {
		next, err := refiner.Refine(ctx, query, current)
		if err != nil {
			return nil, fmt.Errorf("refiner #%d: %w", i, err)
		}
		current = next
	}
	return current, nil
}

// augmentQuery folds the refined documents into the final query.
func (p *Pipeline) augmentQuery(ctx context.Context, query *Query, docs []*document.Document) (*Query, error) {
	return p.queryAugmenter.Augment(ctx, query, docs)
}
