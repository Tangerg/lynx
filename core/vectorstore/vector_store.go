// Package vectorstore defines the interfaces and request shapes for
// vector-indexed document stores (Pinecone, Weaviate, Milvus, Qdrant,
// Chroma, ...). The metadata filter language used by [RetrievalRequest]
// and [DeleteRequest] lives in [filter] and its sub-packages.
package vectorstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
)

// Sentinel errors for the request-shape validators. Callers can match
// these with [errors.Is] to distinguish "caller didn't fill the
// struct" from store-side failures.
var (
	// ErrNilRequest is returned by every Validate when the request
	// pointer is nil.
	ErrNilRequest = errors.New("vectorstore: request must not be nil")

	// ErrEmptyDocuments is returned by [CreateRequest.Validate] on an
	// empty document slice.
	ErrEmptyDocuments = errors.New("vectorstore: Documents must not be empty")

	// ErrMissingFilter is returned by [DeleteRequest.Validate] when no
	// filter expression has been supplied.
	ErrMissingFilter = errors.New("vectorstore: Filter is required")
)

// Similarity-score range for [RetrievalRequest.MinScore] and search
// defaults. Most providers normalize their scores to [0, 1]; the
// constants make that contract explicit.
const (
	// DefaultTopK is the fallback value for [RetrievalRequest.TopK].
	DefaultTopK = 5

	// MinSimilarityScore is the lowest valid score.
	MinSimilarityScore = 0.0

	// MaxSimilarityScore is the highest valid score.
	MaxSimilarityScore = 1.0

	// AcceptAllScores keeps every result regardless of score; alias for
	// [MinSimilarityScore].
	AcceptAllScores = MinSimilarityScore
)

// RetrievalRequest is the input to [Retriever.Retrieve]. Build one with
// [NewRetrievalRequest], then chain WithXxx methods to configure top-k,
// min-score, and metadata filtering.
//
// Example:
//
//	req, err := vectorstore.NewRetrievalRequest("hello world")
//	req.WithTopK(20).WithMinScore(0.7).WithFilter(myFilter)
type RetrievalRequest struct {
	// Query is the search text. Required.
	Query string

	// TopK caps the number of results. Defaults to [DefaultTopK]; must
	// be > 0.
	TopK int

	// MinScore filters out results below this similarity threshold.
	// Range [0.0, 1.0]. Use [AcceptAllScores] to disable.
	MinScore float64

	// Filter is an optional AST expression for metadata filtering.
	Filter ast.Expr
}

// NewRetrievalRequest builds a [RetrievalRequest] with default top-k
// and "accept all scores". Returns an error when validation fails.
func NewRetrievalRequest(text string) (*RetrievalRequest, error) {
	req := &RetrievalRequest{
		Query:    text,
		TopK:     DefaultTopK,
		MinScore: AcceptAllScores,
	}
	return req, req.Validate()
}

// WithTopK sets the result cap. Non-positive values are ignored.
func (r *RetrievalRequest) WithTopK(k int) *RetrievalRequest {
	if k > 0 {
		r.TopK = k
	}
	return r
}

// WithMinScore sets the score threshold. Out-of-range values are
// ignored.
func (r *RetrievalRequest) WithMinScore(score float64) *RetrievalRequest {
	if score >= MinSimilarityScore && score <= MaxSimilarityScore {
		r.MinScore = score
	}
	return r
}

// WithFilter installs a metadata filter expression. nil is ignored —
// pass it explicitly to clear an existing filter via direct field
// access.
func (r *RetrievalRequest) WithFilter(filter ast.Expr) *RetrievalRequest {
	if filter != nil {
		r.Filter = filter
	}
	return r
}

// Validate enforces the request invariants and runs static analysis on
// the filter expression. The first failure is returned.
func (r *RetrievalRequest) Validate() error {
	if r == nil {
		return ErrNilRequest
	}
	if r.Query == "" {
		return errors.New("vectorstore.RetrievalRequest: Query must not be empty")
	}
	if r.TopK <= 0 {
		return fmt.Errorf("vectorstore.RetrievalRequest: TopK must be > 0, got %d", r.TopK)
	}
	if r.MinScore < MinSimilarityScore || r.MinScore > MaxSimilarityScore {
		return fmt.Errorf("vectorstore.RetrievalRequest: MinScore must be in [%.1f, %.1f], got %f",
			MinSimilarityScore, MaxSimilarityScore, r.MinScore)
	}

	if r.Filter != nil {
		if err := filter.Analyze(r.Filter); err != nil {
			return fmt.Errorf("vectorstore.RetrievalRequest: filter analysis: %w", err)
		}
	}
	return nil
}

// Retriever pulls documents similar to a query out of a vector store.
// Results are ranked by similarity score in descending order.
type Retriever interface {
	// Retrieve returns the documents matching request.
	//
	// Implementations honor:
	//   - the score threshold ([RetrievalRequest.MinScore]),
	//   - the metadata filter ([RetrievalRequest.Filter]),
	//   - the result cap ([RetrievalRequest.TopK]).
	Retrieve(ctx context.Context, request *RetrievalRequest) ([]*document.Document, error)
}

// CreateRequest is the input to [Creator.Create]: the documents to
// embed, index, and store.
type CreateRequest struct {
	// Documents is the list to ingest. Required, non-empty.
	Documents []*document.Document
}

// NewCreateRequest builds a [CreateRequest]. Returns an error when
// validation fails.
func NewCreateRequest(docs []*document.Document) (*CreateRequest, error) {
	req := &CreateRequest{Documents: docs}
	return req, req.Validate()
}

// Validate enforces the request invariants.
func (r *CreateRequest) Validate() error {
	if r == nil {
		return ErrNilRequest
	}
	if len(r.Documents) == 0 {
		return ErrEmptyDocuments
	}
	return nil
}

// Creator embeds and indexes documents in the vector store. The store
// runs:
//
//  1. Embedding (text → vector)
//  2. Indexing (vector + metadata → searchable record)
//  3. Storage (record → durable backend)
type Creator interface {
	// Create persists the documents in the request.
	Create(ctx context.Context, request *CreateRequest) error
}

// DeleteRequest is the input to [Deleter.Delete]: a metadata filter
// expression selecting the documents to remove.
type DeleteRequest struct {
	// Filter selects the documents to delete. Required.
	Filter ast.Expr
}

// NewDeleteRequest builds a [DeleteRequest]. Returns an error when
// validation fails.
func NewDeleteRequest(filter ast.Expr) (*DeleteRequest, error) {
	req := &DeleteRequest{Filter: filter}
	return req, req.Validate()
}

// Validate enforces the request invariants and runs static analysis on
// the filter expression.
func (r *DeleteRequest) Validate() error {
	if r == nil {
		return ErrNilRequest
	}
	if r.Filter == nil {
		return ErrMissingFilter
	}

	if err := filter.Analyze(r.Filter); err != nil {
		return fmt.Errorf("vectorstore.DeleteRequest: filter analysis: %w", err)
	}
	return nil
}

// Deleter removes documents matching a metadata filter from the vector
// store.
type Deleter interface {
	// Delete removes every document matching the request's filter.
	Delete(ctx context.Context, request *DeleteRequest) error
}

// VectorStore is the union of [Creator], [Retriever], and [Deleter]
// plus an [VectorStore.Info] accessor for provider identity. Concrete
// providers live in /vectorstores/<provider>.
type VectorStore interface {
	Creator
	Retriever
	Deleter

	// Info returns identity metadata about this store implementation.
	Info() StoreInfo
}

// StoreInfo holds identity metadata for a [VectorStore]. NativeClient
// gives callers access to provider-specific operations the framework
// doesn't surface.
type StoreInfo struct {
	// NativeClient is the underlying provider client (e.g.
	// *pinecone.Client, *weaviate.Client, *qdrant.Client).
	NativeClient any

	// Provider names the backend ("pinecone", "qdrant", "weaviate", ...)
	// — lowercase by convention.
	Provider string
}

// writeFunc is the function-shaped adapter used by [NewDocumentWriter].
type writeFunc func(ctx context.Context, docs []*document.Document) error

// Write implements [document.Writer] for [writeFunc].
func (w writeFunc) Write(ctx context.Context, docs []*document.Document) error {
	return w(ctx, docs)
}

// NewDocumentWriter wraps a [Creator] as a [document.Writer], so the
// vector store fits into pipelines built from generic document
// reader/writer interfaces.
//
// Example:
//
//	writer := vectorstore.NewDocumentWriter(myVectorStore)
//	err := writer.Write(ctx, documents)
func NewDocumentWriter(creator Creator) document.Writer {
	return writeFunc(func(ctx context.Context, docs []*document.Document) error {
		req, err := NewCreateRequest(docs)
		if err != nil {
			return fmt.Errorf("vectorstore.NewDocumentWriter: %w", err)
		}
		return creator.Create(ctx, req)
	})
}
