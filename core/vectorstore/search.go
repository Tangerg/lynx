package vectorstore

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/vectorstore/filter"
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
	Query    string           `json:"query,omitempty"`
	TopK     int              `json:"top_k,omitempty"`
	MinScore float64          `json:"min_score,omitempty"`
	Filter   filter.Predicate `json:"-"`
}

func (r SearchRequest) Validate() error {
	if r.Query == "" {
		return errors.New("vectorstore.SearchRequest: Query must not be empty")
	}
	if r.TopK <= 0 {
		return fmt.Errorf("vectorstore.SearchRequest: TopK must be > 0, got %d", r.TopK)
	}
	if math.IsNaN(r.MinScore) || r.MinScore < MinSimilarityScore || r.MinScore > MaxSimilarityScore {
		return fmt.Errorf("vectorstore.SearchRequest: MinScore must be in [%.1f, %.1f], got %f",
			MinSimilarityScore, MaxSimilarityScore, r.MinScore)
	}

	if r.Filter != nil {
		if err := filter.Validate(r.Filter); err != nil {
			return fmt.Errorf("vectorstore.SearchRequest: filter validation: %w", err)
		}
	}
	return nil
}

// ValidateMatches verifies that a successful Search result honors this
// request's score range, threshold, ordering, and result cap.
func (r SearchRequest) ValidateMatches(matches []Match) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if len(matches) > r.TopK {
		return fmt.Errorf("vectorstore.SearchRequest: got %d matches, TopK is %d", len(matches), r.TopK)
	}
	for i := range matches {
		match := matches[i]
		if err := match.Document.Validate(); err != nil {
			return fmt.Errorf("vectorstore.SearchRequest: matches[%d]: %w", i, err)
		}
		if math.IsNaN(match.Score) || math.IsInf(match.Score, 0) ||
			match.Score < MinSimilarityScore || match.Score > MaxSimilarityScore {
			return fmt.Errorf("vectorstore.SearchRequest: matches[%d] score must be finite and in [%.1f, %.1f], got %v",
				i, MinSimilarityScore, MaxSimilarityScore, match.Score)
		}
		if match.Score < r.MinScore {
			return fmt.Errorf("vectorstore.SearchRequest: matches[%d] score %v is below MinScore %v", i, match.Score, r.MinScore)
		}
		if i > 0 && matches[i-1].Score < match.Score {
			return fmt.Errorf("vectorstore.SearchRequest: matches are not sorted by descending score at index %d", i)
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
