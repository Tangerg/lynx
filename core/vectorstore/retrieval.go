package vectorstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
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
	Query    string   `json:"query,omitempty"`
	TopK     int      `json:"top_k,omitempty"`
	MinScore float64  `json:"min_score,omitempty"`
	Filter   ast.Expr `json:"-"`
}

// NewRetrievalRequest builds a [RetrievalRequest] with default top-k
// and "accept all scores". Returns an error when validation fails.
func NewRetrievalRequest(text string) (*RetrievalRequest, error) {
	req := &RetrievalRequest{
		Query:    text,
		TopK:     DefaultTopK,
		MinScore: AcceptAllScores,
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	return req, nil
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

// Match relates a document to one search operation. Score is deliberately
// kept outside document.Document: relevance belongs to a query/result pair,
// not to the indexed content itself.
type Match struct {
	Document *document.Document `json:"document"`
	Score    float64            `json:"score"`
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
	Retrieve(ctx context.Context, request *RetrievalRequest) ([]Match, error)
}
