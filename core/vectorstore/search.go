package vectorstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
)

// Similarity-score range for [SearchRequest.MinScore] and search
// defaults. Most providers normalize their scores to [0, 1]; the
// constants make that contract explicit.
const (
	// DefaultTopK is the recommended value for [SearchRequest.TopK].
	DefaultTopK = 5

	// MinSimilarityScore is the lowest valid score.
	MinSimilarityScore = 0.0

	// MaxSimilarityScore is the highest valid score.
	MaxSimilarityScore = 1.0

	// AcceptAllScores keeps every result regardless of score; alias for
	// [MinSimilarityScore].
	AcceptAllScores = MinSimilarityScore
)

// SearchRequest describes one semantic search. It is an ordinary value: callers
// set every policy explicitly and call [SearchRequest.Validate] before I/O.
//
// Example:
//
//	req := vectorstore.SearchRequest{
//	    Query: "hello world", TopK: 20, MinScore: 0.7, Filter: myFilter,
//	}
//	err := req.Validate()
type SearchRequest struct {
	Query    string   `json:"query,omitempty"`
	TopK     int      `json:"top_k,omitempty"`
	MinScore float64  `json:"min_score,omitempty"`
	Filter   ast.Expr `json:"-"`
}

func (r SearchRequest) Validate() error {
	if r.Query == "" {
		return errors.New("vectorstore.SearchRequest: Query must not be empty")
	}
	if r.TopK <= 0 {
		return fmt.Errorf("vectorstore.SearchRequest: TopK must be > 0, got %d", r.TopK)
	}
	if r.MinScore < MinSimilarityScore || r.MinScore > MaxSimilarityScore {
		return fmt.Errorf("vectorstore.SearchRequest: MinScore must be in [%.1f, %.1f], got %f",
			MinSimilarityScore, MaxSimilarityScore, r.MinScore)
	}

	if r.Filter != nil {
		if err := filter.Analyze(r.Filter); err != nil {
			return fmt.Errorf("vectorstore.SearchRequest: filter analysis: %w", err)
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

// Searcher pulls documents similar to a query out of a vector store.
// Results are ranked by similarity score in descending order.
type Searcher interface {
	// Search returns the documents matching request.
	//
	// Implementations honor:
	//   - the score threshold ([SearchRequest.MinScore]),
	//   - the metadata filter ([SearchRequest.Filter]),
	//   - the result cap ([SearchRequest.TopK]).
	Search(ctx context.Context, request SearchRequest) ([]Match, error)
}
