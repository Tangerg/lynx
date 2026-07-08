package rag

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/core/document"
	corevs "github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
)

// VectorStoreFilterKey is the [Query.Extra] metadata key under which a caller
// may stash a per-call filter expression — either as a parsed
// [ast.Expr] or as a raw filter-DSL string. The retriever consults
// this slot before falling back to the configured FilterFunc.
const VectorStoreFilterKey = "lynx:ai:rag:retriever:filter_expr"

type VectorStoreConfig struct {
	// VectorStore performs the actual similarity search. Required.
	VectorStore corevs.Retriever

	// TopK caps the number of returned documents. Non-positive values
	// fall back to [corevs.DefaultTopK].
	TopK int

	// MinScore filters out matches below this similarity threshold.
	// Range [0.0, 1.0].
	MinScore float64

	// FilterFunc dynamically builds a metadata filter from the query's
	// Extra map. Optional; when both [VectorStoreFilterKey] is set on the
	// query and FilterFunc is provided, the per-query filter wins.
	FilterFunc func(ctx context.Context, params map[string]any) (ast.Expr, error)
}

func (c *VectorStoreConfig) validate() error {
	if c.VectorStore == nil {
		return errors.New("rag.VectorStoreConfig: VectorStore is required")
	}
	if c.TopK < 0 {
		return errors.New("rag.VectorStoreConfig: TopK must be >= 0")
	}
	if c.MinScore < corevs.MinSimilarityScore || c.MinScore > corevs.MaxSimilarityScore {
		return fmt.Errorf("rag.VectorStoreConfig: MinScore must be in [%.1f, %.1f]",
			corevs.MinSimilarityScore, corevs.MaxSimilarityScore)
	}
	return nil
}

func (c *VectorStoreConfig) applyDefaults() {
	if c.TopK == 0 {
		c.TopK = corevs.DefaultTopK
	}
}

var _ Retriever = (*vectorStoreRetriever)(nil)

type vectorStoreRetriever struct {
	vectorStore corevs.Retriever
	topK        int
	minScore    float64
	filterFunc  func(ctx context.Context, params map[string]any) (ast.Expr, error)
}

// NewVectorStoreRetriever returns a [Retriever] backed by a core vector store.
// It supports per-query metadata filters via [VectorStoreFilterKey],
// configured filters via [VectorStoreConfig.FilterFunc], top-K capping, and
// similarity thresholds.
func NewVectorStoreRetriever(cfg VectorStoreConfig) (Retriever, error) {
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &vectorStoreRetriever{
		vectorStore: cfg.VectorStore,
		topK:        cfg.TopK,
		minScore:    cfg.MinScore,
		filterFunc:  cfg.FilterFunc,
	}, nil
}

// Retrieve issues a similarity search via the underlying vector store.
func (v *vectorStoreRetriever) Retrieve(ctx context.Context, query *Query) ([]*document.Document, error) {
	if query == nil {
		return nil, ErrNilQuery
	}

	request, err := corevs.NewRetrievalRequest(query.Text)
	if err != nil {
		return nil, err
	}

	expr, err := v.resolveFilter(ctx, query)
	if err != nil {
		return nil, err
	}

	request.WithTopK(v.topK).WithMinScore(v.minScore).WithFilter(expr)

	return v.vectorStore.Retrieve(ctx, request)
}

// resolveFilter picks the filter expression to use for this call,
// preferring the per-query [VectorStoreFilterKey] slot over the configured
// FilterFunc. Returns nil, nil when no filter applies.
func (v *vectorStoreRetriever) resolveFilter(ctx context.Context, query *Query) (ast.Expr, error) {
	if value, exists := query.Get(VectorStoreFilterKey); exists {
		switch typed := value.(type) {
		case string:
			return filter.Parse(typed)
		case ast.Expr:
			return typed, nil
		}
	}

	if v.filterFunc != nil {
		return v.filterFunc(ctx, query.Extra())
	}
	return nil, nil
}
