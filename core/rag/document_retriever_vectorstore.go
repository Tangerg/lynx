package rag

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
)

// FilterExprKey is the [Query.Extra] metadata key under which a caller
// may stash a per-call filter expression — either as a parsed
// [ast.Expr] or as a raw filter-DSL string. The retriever consults
// this slot before falling back to the configured FilterFunc.
const FilterExprKey = "lynx:ai:rag:retriever:filter_expr"

// VectorStoreDocumentRetrieverConfig configures a
// [VectorStoreDocumentRetriever].
type VectorStoreDocumentRetrieverConfig struct {
	// VectorStore performs the actual similarity search. Required.
	VectorStore vectorstore.Retriever

	// TopK caps the number of returned documents. Non-positive values
	// fall back to [vectorstore.DefaultTopK].
	TopK int

	// MinScore filters out matches below this similarity threshold.
	// Range [0.0, 1.0].
	MinScore float64

	// FilterFunc dynamically builds a metadata filter from the query's
	// Extra map. Optional; when both [FilterExprKey] is set on the
	// query and FilterFunc is provided, the per-query filter wins.
	FilterFunc func(ctx context.Context, params map[string]any) (ast.Expr, error)
}

// validate fills defaults and rejects invalid configurations.
func (c *VectorStoreDocumentRetrieverConfig) validate() error {
	if c == nil {
		return errors.New("rag.VectorStoreDocumentRetrieverConfig: config must not be nil")
	}
	if c.VectorStore == nil {
		return errors.New("rag.VectorStoreDocumentRetrieverConfig: VectorStore is required")
	}
	if c.TopK < 0 {
		return errors.New("rag.VectorStoreDocumentRetrieverConfig: TopK must be ≥ 0")
	}
	if c.TopK == 0 {
		c.TopK = vectorstore.DefaultTopK
	}
	if c.MinScore < vectorstore.MinSimilarityScore || c.MinScore > vectorstore.MaxSimilarityScore {
		return fmt.Errorf("rag.VectorStoreDocumentRetrieverConfig: MinScore must be in [%.1f, %.1f]",
			vectorstore.MinSimilarityScore, vectorstore.MaxSimilarityScore)
	}
	return nil
}

var _ DocumentRetriever = (*VectorStoreDocumentRetriever)(nil)

// VectorStoreDocumentRetriever bridges the RAG retrieval interface and
// a [vectorstore.Retriever]. It supports per-call metadata filters
// (either parsed expressions stashed under [FilterExprKey] or built
// dynamically via FilterFunc), top-K capping, and similarity
// thresholds.
//
// Example:
//
//	r, err := rag.NewVectorStoreDocumentRetriever(&rag.VectorStoreDocumentRetrieverConfig{
//	    VectorStore: store,
//	    TopK:        10,
//	    MinScore:    0.7,
//	})
type VectorStoreDocumentRetriever struct {
	vectorStore vectorstore.Retriever
	topK        int
	minScore    float64
	filterFunc  func(ctx context.Context, params map[string]any) (ast.Expr, error)
}

// NewVectorStoreDocumentRetriever builds a
// [VectorStoreDocumentRetriever]. Returns an error when the
// configuration fails [VectorStoreDocumentRetrieverConfig.validate].
func NewVectorStoreDocumentRetriever(cfg *VectorStoreDocumentRetrieverConfig) (*VectorStoreDocumentRetriever, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &VectorStoreDocumentRetriever{
		vectorStore: cfg.VectorStore,
		topK:        cfg.TopK,
		minScore:    cfg.MinScore,
		filterFunc:  cfg.FilterFunc,
	}, nil
}

// Retrieve issues a similarity search via the underlying vector store.
func (v *VectorStoreDocumentRetriever) Retrieve(ctx context.Context, query *Query) ([]*document.Document, error) {
	if query == nil {
		return nil, ErrNilQuery
	}

	request, err := vectorstore.NewRetrievalRequest(query.Text)
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
// preferring the per-query [FilterExprKey] slot over the configured
// FilterFunc. Returns nil, nil when no filter applies.
func (v *VectorStoreDocumentRetriever) resolveFilter(ctx context.Context, query *Query) (ast.Expr, error) {
	if value, exists := query.Get(FilterExprKey); exists {
		switch typed := value.(type) {
		case string:
			return filter.Parse(typed)
		case ast.Expr:
			return typed, nil
		}
	}

	if v.filterFunc != nil {
		return v.filterFunc(ctx, query.Extra)
	}
	return nil, nil
}
