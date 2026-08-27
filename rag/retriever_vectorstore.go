package rag

import (
	"context"
	"errors"
	"fmt"

	"github.com/samber/lo"

	corevs "github.com/Tangerg/scope/core/vectorstore"
	"github.com/Tangerg/scope/core/vectorstore/filter"
)

var vectorStoreFilterValueKey = mustValueKey[filter.Predicate]("vector store filter")

// VectorStoreFilterValueKey returns the typed query slot for a parsed per-call
// filter. Parse textual filter DSL with [filter.Parse] before attaching it.
func VectorStoreFilterValueKey() ValueKey[filter.Predicate] { return vectorStoreFilterValueKey }

// VectorStoreRetrieverConfig configures [NewVectorStoreRetriever].
type VectorStoreRetrieverConfig struct {
	// VectorStore performs the actual similarity search. Required.
	VectorStore corevs.Searcher

	// TopK caps the number of returned documents. Zero uses
	// [corevs.DefaultTopK]; negative values are invalid.
	TopK int

	// MinScore filters out matches below this similarity threshold.
	// Range [0.0, 1.0].
	MinScore corevs.Score

	// FilterFunc dynamically builds a metadata filter from the complete query.
	// Optional; when [VectorStoreFilterValueKey] is set, the per-query filter wins.
	FilterFunc func(ctx context.Context, query Query) (filter.Predicate, error)
}

func (c VectorStoreRetrieverConfig) normalized() (VectorStoreRetrieverConfig, error) {
	if lo.IsNil(c.VectorStore) {
		return VectorStoreRetrieverConfig{}, errors.New("rag: vector store is required")
	}
	if c.TopK < 0 {
		return VectorStoreRetrieverConfig{}, errors.New("rag: vector-store top K must not be negative")
	}
	if c.MinScore < corevs.MinSimilarityScore || c.MinScore > corevs.MaxSimilarityScore {
		return VectorStoreRetrieverConfig{}, fmt.Errorf(
			"rag: vector-store minimum score must be in [%.1f, %.1f]",
			corevs.MinSimilarityScore,
			corevs.MaxSimilarityScore,
		)
	}
	if c.TopK == 0 {
		c.TopK = corevs.DefaultTopK
	}
	return c, nil
}

var _ Retriever = (*VectorStoreRetriever)(nil)

// VectorStoreRetriever retrieves candidates from a core vector store.
type VectorStoreRetriever struct {
	vectorStore corevs.Searcher
	topK        int
	minScore    corevs.Score
	filterFunc  func(ctx context.Context, query Query) (filter.Predicate, error)
}

// NewVectorStoreRetriever returns a [VectorStoreRetriever] backed by a core
// vector store.
// It supports per-query metadata filters via [VectorStoreFilterValueKey],
// configured filters via [VectorStoreRetrieverConfig.FilterFunc], top-K
// capping, and
// similarity thresholds.
func NewVectorStoreRetriever(config VectorStoreRetrieverConfig) (*VectorStoreRetriever, error) {
	config, err := config.normalized()
	if err != nil {
		return nil, err
	}

	return &VectorStoreRetriever{
		vectorStore: config.VectorStore,
		topK:        config.TopK,
		minScore:    config.MinScore,
		filterFunc:  config.FilterFunc,
	}, nil
}

// Retrieve issues a similarity search via the underlying vector store.
func (v *VectorStoreRetriever) Retrieve(ctx context.Context, query Query) (Candidates, error) {
	if err := query.Validate(); err != nil {
		return nil, err
	}

	expr, err := v.resolveFilter(ctx, query)
	if err != nil {
		return nil, err
	}

	request := &corevs.SearchRequest{
		Query: query.Text(),
		Options: corevs.SearchOptions{
			TopK: v.topK, MinScore: v.minScore, Filter: expr,
		},
	}
	if validateErr := request.Validate(); validateErr != nil {
		return nil, fmt.Errorf("rag: build vector-store request: %w", validateErr)
	}
	response, err := v.vectorStore.Search(ctx, request)
	if err != nil {
		return nil, err
	}
	if err := response.ValidateFor(request); err != nil {
		return nil, fmt.Errorf("rag: vector-store response: %w", err)
	}
	candidates := make(Candidates, 0, len(response.Results))
	for _, result := range response.Results {
		candidates = append(candidates, Candidate{Document: result.Document, Score: result.Score.Float64()})
	}
	return candidates, nil
}

// resolveFilter picks the filter expression to use for this call,
// preferring the per-query [VectorStoreFilterValueKey] slot over the configured
// FilterFunc. Returns nil, nil when no filter applies.
func (v *VectorStoreRetriever) resolveFilter(ctx context.Context, query Query) (filter.Predicate, error) {
	expression, exists, err := query.Value(vectorStoreFilterValueKey)
	if err != nil {
		return nil, fmt.Errorf("rag: read vector-store filter: %w", err)
	}
	if exists {
		return expression, nil
	}

	if v.filterFunc != nil {
		return v.filterFunc(ctx, query)
	}
	return nil, nil
}
