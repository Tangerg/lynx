package vectorstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/ai/media/document"
	"github.com/Tangerg/lynx/ai/vectorstore/filter"
	"github.com/Tangerg/lynx/ai/vectorstore/filter/ast"
)

const (
	// DefaultTopK is the default maximum number of documents to return in similarity search.
	DefaultTopK = 5

	// MinSimilarityScore is the minimum valid similarity score.
	// Scores below this threshold are considered invalid.
	MinSimilarityScore = 0.0

	// MaxSimilarityScore is the maximum valid similarity score.
	// Scores above this threshold are considered invalid.
	MaxSimilarityScore = 1.0

	// AcceptAllScores is a special threshold that accepts all results regardless of similarity score.
	// Use this when you want to retrieve results without score filtering.
	AcceptAllScores = MinSimilarityScore
)

// RetrievalRequest specifies parameters for retrieving documents from vector stores.
// It supports both text-based and vector-based queries with configurable result filtering.
type RetrievalRequest struct {
	// Query is the text that defines the search input.
	Query string

	// TopK is the maximum number of documents to return, ranked by similarity score.
	// Must be greater than 0. Defaults to DefaultTopK (5) if not specified.
	TopK int

	// MinScore is the minimum similarity score threshold for filtering results.
	// Only documents with similarity score >= MinScore will be returned.
	// Valid range: [0.0, 1.0]. Use AcceptAllScores (0.0) to accept all results.
	MinScore float64

	// Filter is an optional AST expression for metadata-based filtering.
	// Use this to filter documents based on their metadata fields.
	// If nil, no metadata filtering is applied.
	Filter ast.Expr
}

// NewRetrievalRequest creates a new retrieval request with a text query.
// The request is initialized with default values (TopK=5, MinScore=0.0, no filter).
//
// Parameters:
//   - text: The search text, must not be empty
//
// Returns an error if validation fails.
func NewRetrievalRequest(text string) (*RetrievalRequest, error) {
	req := &RetrievalRequest{
		Query:    text,
		TopK:     DefaultTopK,
		MinScore: AcceptAllScores,
	}
	return req, req.Validate()
}

// WithTopK sets the maximum number of results to return.
// If k <= 0, the value is ignored and the request remains unchanged.
// This method supports method chaining for convenient configuration.
//
// Example:
//
//	request.WithTopK(10).WithMinScore(0.7)
func (r *RetrievalRequest) WithTopK(k int) *RetrievalRequest {
	if k > 0 {
		r.TopK = k
	}

	return r
}

// WithMinScore sets the minimum similarity score threshold.
// If score is outside the valid range [0.0, 1.0], the value is ignored.
// This method supports method chaining for convenient configuration.
//
// Example:
//
//	request.WithMinScore(0.8).WithTopK(10)
func (r *RetrievalRequest) WithMinScore(score float64) *RetrievalRequest {
	if score >= MinSimilarityScore &&
		score <= MaxSimilarityScore {
		r.MinScore = score
	}

	return r
}

// WithFilter sets the metadata filter expression for result filtering.
// If filter is nil, the value is ignored and the request remains unchanged.
// This method supports method chaining for convenient configuration.
//
// Example:
//
//	request.WithFilter(filter.EQ("category", "tech")).WithTopK(10)
func (r *RetrievalRequest) WithFilter(filter ast.Expr) *RetrievalRequest {
	if filter != nil {
		r.Filter = filter
	}

	return r
}

// Validate checks if the request parameters are valid.
// It validates:
//   - Query is not empty
//   - TopK is greater than 0
//   - MinScore is within [0.0, 1.0]
//   - Filter expression is syntactically correct (if present)
//
// Returns an error describing the first validation failure encountered.
func (r *RetrievalRequest) Validate() error {
	if r == nil {
		return errors.New("request cannot be nil")
	}

	if r.Query == "" {
		return errors.New("query text cannot be empty")
	}

	if r.TopK <= 0 {
		return errors.New("topK must be greater than 0")
	}

	if r.MinScore < MinSimilarityScore || r.MinScore > MaxSimilarityScore {
		return errors.New("minScore must be between 0.0 and 1.0")
	}

	if r.Filter != nil {
		err := filter.Analyze(r.Filter)
		if err != nil {
			return err
		}
	}

	return nil
}

// Retriever retrieves semantically relevant documents from vector stores.
// Implementations should support both text and vector queries, and apply
// all specified filters (similarity threshold and metadata filtering).
type Retriever interface {
	// Retrieve finds documents similar to the query based on vector similarity.
	// Documents are ranked by similarity score in descending order.
	//
	// The returned documents will:
	//   - Have similarity scores >= request.MinScore
	//   - Match the metadata filter expression (if specified)
	//   - Be limited to at most request.TopK results
	//
	// Returns an error if:
	//   - The request validation fails
	//   - The vector store is unavailable
	//   - The query processing fails (e.g., embedding generation fails)
	Retrieve(ctx context.Context, request *RetrievalRequest) ([]*document.Document, error)
}

// CreateRequest specifies parameters for creating documents in the vector store.
// Documents will be embedded and indexed for similarity search.
type CreateRequest struct {
	// Documents is the list of documents to create.
	// Each document must have content and optionally metadata.
	// The vector store will generate embeddings for the content.
	Documents []*document.Document
}

// NewCreateRequest creates a new create request with the given documents.
//
// Parameters:
//   - docs: The documents to create, must not be empty
//
// Returns an error if validation fails.
func NewCreateRequest(docs []*document.Document) (*CreateRequest, error) {
	req := &CreateRequest{
		Documents: docs,
	}
	return req, req.Validate()
}

// Validate checks if the request parameters are valid.
// It ensures that the documents list is not empty.
//
// Returns an error if validation fails.
func (r *CreateRequest) Validate() error {
	if r == nil {
		return errors.New("request cannot be nil")
	}

	if len(r.Documents) == 0 {
		return errors.New("documents list cannot be empty")
	}

	return nil
}

// Creator creates and stores documents in the vector store.
// Implementations should handle embedding generation and indexing.
type Creator interface {
	// Create stores documents in the vector store.
	// The documents will be:
	//   1. Embedded (converted to vector representations)
	//   2. Indexed for efficient similarity search
	//   3. Stored with their metadata
	//
	// Returns an error if:
	//   - The request validation fails
	//   - The vector store is unavailable
	//   - Embedding generation fails
	//   - Storage or indexing fails
	Create(ctx context.Context, request *CreateRequest) error
}

// DeleteRequest specifies parameters for deleting documents from the vector store.
// Documents are selected for deletion based on metadata filtering.
type DeleteRequest struct {
	// Filter specifies which documents to delete using an AST expression.
	// Documents matching this filter will be removed from the vector store.
	// Must not be nil - use a filter expression to specify deletion criteria.
	Filter ast.Expr
}

// NewDeleteRequest creates a new delete request with the given filter.
// The filter expression defines which documents should be deleted based on their metadata.
//
// Parameters:
//   - filter: The filter expression, must not be nil
//
// Returns an error if validation fails.
func NewDeleteRequest(filter ast.Expr) (*DeleteRequest, error) {
	req := &DeleteRequest{
		Filter: filter,
	}

	return req, req.Validate()
}

// Validate checks if the request parameters are valid.
// It ensures that:
//   - Filter is not nil
//   - Filter expression is syntactically correct
//
// Returns an error if validation fails.
func (r *DeleteRequest) Validate() error {
	if r == nil {
		return errors.New("request cannot be nil")
	}

	if r.Filter == nil {
		return errors.New("filter cannot be nil, specify a filter expression to select documents for deletion")
	}

	return filter.Analyze(r.Filter)
}

// Deleter deletes documents from the vector store.
// Implementations should support metadata-based filtering for selective deletion.
type Deleter interface {
	// Delete removes documents matching the filter criteria.
	// All documents whose metadata matches the filter expression will be deleted.
	//
	// Returns an error if:
	//   - The request validation fails
	//   - The vector store is unavailable
	//   - The filter expression is invalid
	//   - The deletion operation fails
	Delete(ctx context.Context, request *DeleteRequest) error
}

// VectorStore is a comprehensive interface that combines document creation,
// retrieval, and deletion operations for vector stores.
// It provides a complete solution for managing vector-indexed documents.
type VectorStore interface {
	Creator
	Retriever
	Deleter

	// Info returns metadata about the store implementation.
	// Use this to access provider-specific information or the native client.
	Info() StoreInfo
}

// StoreInfo contains metadata about the vector store implementation.
// It provides information about the underlying provider and access to the native client.
type StoreInfo struct {
	// NativeClient is the underlying client used by the store.
	// This can be used for advanced operations not covered by the VectorStore interface.
	// The type depends on the specific vector store provider.
	// Example: *pinecone.Client, *weaviate.Client, *milvus.Client
	NativeClient any

	// Provider identifies the vector store provider.
	// Common values: "pinecone", "weaviate", "milvus", "qdrant", "chroma"
	Provider string
}

type writeFunc func(ctx context.Context, docs []*document.Document) error

func (w writeFunc) Write(ctx context.Context, docs []*document.Document) error {
	return w(ctx, docs)
}

// NewDocumentWriter creates a document.Writer adapter from a Creator.
// This adapter allows a Creator to be used wherever a document.Writer is expected.
//
// The returned Writer will:
//   - Wrap documents in a CreateRequest
//   - Delegate to the Creator's Create method
//   - Propagate any errors from the Create operation
//
// Example:
//
//	writer := NewDocumentWriter(myVectorStore)
//	err := writer.Write(ctx, documents)
func NewDocumentWriter(creator Creator) document.Writer {
	return writeFunc(func(ctx context.Context, docs []*document.Document) error {
		req, err := NewCreateRequest(docs)
		if err != nil {
			return fmt.Errorf("invalid document write request: %w", err)
		}

		return creator.Create(ctx, req)
	})
}
