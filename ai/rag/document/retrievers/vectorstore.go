package retrievers

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/ai/media/document"
	"github.com/Tangerg/lynx/ai/rag"
	"github.com/Tangerg/lynx/ai/vectorstore"
	"github.com/Tangerg/lynx/ai/vectorstore/filter"
	"github.com/Tangerg/lynx/ai/vectorstore/filter/ast"
)

const (
	// FilterExprKey is the metadata key for filter ast.Expr.
	FilterExprKey = "lynx:ai:rag:retriever:filter_expr"
)

// VectorStoreRetrieverConfig holds the configuration for VectorStoreRetriever.
type VectorStoreRetrieverConfig struct {
	// VectorStore is the vector store used for document retrieval.
	// Required.
	VectorStore vectorstore.Retriever

	// TopK specifies the maximum number of documents to retrieve.
	// Optional. Defaults to vectorstore.DefaultTopK if not provided or zero.
	// Must be positive.
	TopK int

	// MinScore sets the minimum similarity score threshold for retrieved documents.
	// Optional. Must be between vectorstore.MinSimilarityScore and vectorstore.MaxSimilarityScore.
	// Documents with scores below this threshold will be filtered out.
	MinScore float64

	// FilterFunc is a custom function to build filter expressions dynamically.
	// Optional. If provided, it will be called to generate filter expressions
	// based on the query parameters.
	FilterFunc func(ctx context.Context, params map[string]any) (ast.Expr, error)
}

func (cfg *VectorStoreRetrieverConfig) validate() error {
	if cfg == nil {
		return errors.New("vector store retriever config cannot be nil")
	}

	if cfg.VectorStore == nil {
		return errors.New("vector store retriever config: vector store is required")
	}

	if cfg.TopK < 0 {
		return errors.New("vector store retriever config: top k must be positive")
	}

	if cfg.TopK == 0 {
		cfg.TopK = vectorstore.DefaultTopK
	}

	if cfg.MinScore < vectorstore.MinSimilarityScore || cfg.MinScore > vectorstore.MaxSimilarityScore {
		return errors.New("vector store retriever config: min score must be between min and max similarity score")
	}

	return nil
}

var _ rag.DocumentRetriever = (*VectorStoreRetriever)(nil)

// VectorStoreRetriever retrieves documents from a vector store that are semantically
// similar to the input query. It supports filtering based on metadata, similarity
// threshold, and top-k results.
//
// This retriever is useful for:
//   - Performing semantic search over document embeddings
//   - Filtering results by metadata attributes
//   - Controlling result quality through similarity thresholds
//   - Limiting the number of returned documents for efficiency
type VectorStoreRetriever struct {
	vectorStore vectorstore.Retriever
	topK        int
	minScore    float64
	filterFunc  func(ctx context.Context, params map[string]any) (ast.Expr, error)
}

func NewVectorStoreRetriever(cfg *VectorStoreRetrieverConfig) (*VectorStoreRetriever, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &VectorStoreRetriever{
		vectorStore: cfg.VectorStore,
		topK:        cfg.TopK,
		minScore:    cfg.MinScore,
		filterFunc:  cfg.FilterFunc,
	}, nil
}

func (v *VectorStoreRetriever) Retrieve(ctx context.Context, query *rag.Query) ([]*document.Document, error) {
	if query == nil {
		return nil, errors.New("query cannot be nil")
	}

	request, err := vectorstore.NewRetrievalRequest(query.Text)
	if err != nil {
		return nil, err
	}

	filterExpr, err := v.buildFilterExpression(ctx, query)
	if err != nil {
		return nil, err
	}

	request.
		WithTopK(v.topK).
		WithMinScore(v.minScore).
		WithFilter(filterExpr)

	return v.vectorStore.Retrieve(ctx, request)
}

func (v *VectorStoreRetriever) buildFilterExpression(ctx context.Context, query *rag.Query) (ast.Expr, error) {
	filterValue, exists := query.Get(FilterExprKey)
	if exists {
		switch typedFilter := filterValue.(type) {
		case string:
			return filter.Parse(typedFilter)
		case ast.Expr:
			return typedFilter, nil
		}
	}

	if v.filterFunc != nil {
		return v.filterFunc(ctx, query.Extra)
	}

	return nil, nil
}
