package rag

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/Tangerg/lynx/ai/media/document"
)

// PipelineConfig holds the configuration for creating a RAG pipeline.
// It defines the components that will be used in each stage of the pipeline.
type PipelineConfig struct {
	// QueryTransformers is a list of transformers applied sequentially to the query.
	// Optional: can be nil or empty if no transformation is needed.
	QueryTransformers []QueryTransformer

	// QueryExpander expands a single query into multiple queries.
	// Optional: defaults to Nop if not provided.
	QueryExpander QueryExpander

	// DocumentRetrievers is a list of retrievers executed in parallel.
	// Required: at least one retriever must be provided.
	DocumentRetrievers []DocumentRetriever

	// DocumentRefiners is a list of refiners applied sequentially to documents.
	// Optional: can be nil or empty if no refinement is needed.
	DocumentRefiners []DocumentRefiner

	// QueryAugmenter augments the query with retrieved documents.
	// Optional: defaults to Nop if not provided.
	QueryAugmenter QueryAugmenter
}

// validate checks if the pipeline configuration is valid and applies defaults.
func (c *PipelineConfig) validate() error {
	if c == nil {
		return errors.New("pipeline config cannot be nil")
	}

	if len(c.DocumentRetrievers) == 0 {
		return errors.New("at least one document retriever is required")
	}

	// Apply defaults for optional components
	if c.QueryExpander == nil {
		c.QueryExpander = NewNop()
	}
	if c.QueryAugmenter == nil {
		c.QueryAugmenter = NewNop()
	}

	return nil
}

// Pipeline orchestrates the complete RAG (Retrieval-Augmented Generation) workflow.
// It processes queries through multiple stages: transformation, expansion, retrieval,
// refinement, and augmentation.
type Pipeline struct {
	queryTransformers  []QueryTransformer
	queryExpander      QueryExpander
	documentRetrievers []DocumentRetriever
	documentRefiners   []DocumentRefiner
	queryAugmenter     QueryAugmenter
}

// NewPipeline creates a new RAG pipeline with the given configuration.
// It returns an error if the configuration is invalid.
func NewPipeline(config *PipelineConfig) (*Pipeline, error) {
	if err := config.validate(); err != nil {
		return nil, fmt.Errorf("invalid pipeline config: %w", err)
	}

	return &Pipeline{
		queryTransformers:  config.QueryTransformers,
		queryExpander:      config.QueryExpander,
		documentRetrievers: config.DocumentRetrievers,
		documentRefiners:   config.DocumentRefiners,
		queryAugmenter:     config.QueryAugmenter,
	}, nil
}

// transformQuery applies all registered query transformers sequentially.
func (p *Pipeline) transformQuery(ctx context.Context, query *Query) (*Query, error) {
	current := query

	for i, transformer := range p.queryTransformers {
		transformed, err := transformer.Transform(ctx, current)
		if err != nil {
			return nil, fmt.Errorf("query transformation failed at stage %d: %w", i, err)
		}
		current = transformed
	}

	return current, nil
}

// expandQuery expands a single query into multiple queries for comprehensive retrieval.
func (p *Pipeline) expandQuery(ctx context.Context, query *Query) ([]*Query, error) {
	queries, err := p.queryExpander.Expand(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query expansion failed: %w", err)
	}

	return queries, nil
}

// retrieveByQuery retrieves documents using all configured retrievers in parallel.
func (p *Pipeline) retrieveByQuery(ctx context.Context, query *Query) ([]*document.Document, error) {
	var (
		mu   sync.Mutex
		docs []*document.Document
	)

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(len(p.documentRetrievers))

	for idx, retriever := range p.documentRetrievers {
		g.Go(func() error {
			retrieved, err := retriever.Retrieve(gctx, query)
			if err != nil {
				return fmt.Errorf("retriever %d failed: %w", idx, err)
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
		// Partial failure: return what we have
		return docs, nil
	}

	return docs, nil
}

// retrieveByQueries retrieves documents for multiple queries in parallel.
func (p *Pipeline) retrieveByQueries(ctx context.Context, queries []*Query) ([]*document.Document, error) {
	var (
		mu   sync.Mutex
		docs []*document.Document
		g, _ = errgroup.WithContext(ctx)
	)

	g.SetLimit(len(queries))

	for idx, query := range queries {
		g.Go(func() error {
			retrieved, err := p.retrieveByQuery(ctx, query)
			if err != nil {
				return fmt.Errorf("retrieval for query %d failed: %w", idx, err)
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
		// Partial failure: return what we have
		return docs, nil
	}

	return docs, nil
}

// refineDocuments applies all registered document refiners sequentially.
func (p *Pipeline) refineDocuments(ctx context.Context, query *Query, docs []*document.Document) ([]*document.Document, error) {
	current := docs

	for i, refiner := range p.documentRefiners {
		refined, err := refiner.Refine(ctx, query, current)
		if err != nil {
			return nil, fmt.Errorf("document refinement failed at stage %d: %w", i, err)
		}
		current = refined
	}

	return current, nil
}

// augmentQuery augments the original query with the retrieved and refined documents.
func (p *Pipeline) augmentQuery(ctx context.Context, query *Query, docs []*document.Document) (*Query, error) {
	augmented, err := p.queryAugmenter.Augment(ctx, query, docs)
	if err != nil {
		return nil, fmt.Errorf("query augmentation failed: %w", err)
	}

	return augmented, nil
}

// Execute runs the complete RAG pipeline on the given query.
// It returns the augmented query and refined documents, or an error if any stage fails.
//
// Pipeline stages:
//  1. Transform: Apply query transformations
//  2. Expand: Synthesize multiple query variants
//  3. Retrieve: Fetch documents from all retrievers
//  4. Refine: Filter and rank documents
//  5. Augment: Enhance query with document context
func (p *Pipeline) Execute(ctx context.Context, query *Query) (*Query, []*document.Document, error) {
	// Stage 1: Transform query
	transformed, err := p.transformQuery(ctx, query)
	if err != nil {
		return nil, nil, fmt.Errorf("pipeline stage 'transform' failed: %w", err)
	}

	// Stage 2: Expand query
	expanded, err := p.expandQuery(ctx, transformed)
	if err != nil {
		return nil, nil, fmt.Errorf("pipeline stage 'expand' failed: %w", err)
	}

	// Stage 3: Retrieve documents
	retrieved, err := p.retrieveByQueries(ctx, expanded)
	if err != nil {
		return nil, nil, fmt.Errorf("pipeline stage 'retrieve' failed: %w", err)
	}

	// Stage 4: Refine documents
	refined, err := p.refineDocuments(ctx, query, retrieved)
	if err != nil {
		return nil, nil, fmt.Errorf("pipeline stage 'refine' failed: %w", err)
	}

	// Stage 5: Augment query
	augmented, err := p.augmentQuery(ctx, query, refined)
	if err != nil {
		return nil, nil, fmt.Errorf("pipeline stage 'augment' failed: %w", err)
	}

	return augmented, refined, nil
}

// Run is a convenience method that creates a Query from a text string and executes the pipeline.
func (p *Pipeline) Run(ctx context.Context, text string) (*Query, []*document.Document, error) {
	query, err := NewQuery(text)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create query: %w", err)
	}
	return p.Execute(ctx, query)
}
