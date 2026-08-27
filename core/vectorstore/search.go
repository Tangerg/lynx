package vectorstore

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/scope/core/document"
	"github.com/Tangerg/scope/core/vectorstore/filter"
)

// Similarity-score range for [SearchOptions.MinScore] and search defaults.
const (
	// DefaultTopK is the recommended value for [SearchOptions.TopK].
	DefaultTopK = 5

	// MinSimilarityScore is the lowest valid score.
	MinSimilarityScore = 0.0

	// MaxSimilarityScore is the highest valid score.
	MaxSimilarityScore = 1.0
)

// SearchOptions owns the policies applied to a semantic search.
type SearchOptions struct {
	TopK     int              `json:"top_k,omitempty"`
	MinScore Score            `json:"min_score,omitempty"`
	Filter   filter.Predicate `json:"-"`
}

func NewSearchOptions() SearchOptions {
	return SearchOptions{TopK: DefaultTopK}
}

func (s SearchOptions) Validate() error {
	if s.TopK <= 0 {
		return fmt.Errorf("%w: top K must be greater than zero, got %d", ErrInvalidOptions, s.TopK)
	}
	if err := s.MinScore.Validate(); err != nil {
		return fmt.Errorf("%w: minimum score: %w", ErrInvalidOptions, err)
	}
	if s.Filter != nil {
		if err := s.Filter.Validate(); err != nil {
			return fmt.Errorf("%w: filter: %w", ErrInvalidOptions, err)
		}
	}
	return nil
}

func (s SearchOptions) MarshalJSON() ([]byte, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	type wireSearchOptions SearchOptions
	return json.Marshal(wireSearchOptions(s))
}

func (s *SearchOptions) UnmarshalJSON(data []byte) error {
	if s == nil {
		return fmt.Errorf("%w: search options receiver is nil", ErrInvalidOptions)
	}
	type wireSearchOptions SearchOptions
	var decoded wireSearchOptions
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("%w: decode search options: %w", ErrInvalidOptions, err)
	}
	candidate := SearchOptions(decoded)
	if err := candidate.Validate(); err != nil {
		return err
	}
	*s = candidate
	return nil
}

// SearchRequest describes one semantic search and owns its input validation.
type SearchRequest struct {
	Query   string        `json:"query,omitempty"`
	Options SearchOptions `json:"options"`
}

func NewSearchRequest(query string) (*SearchRequest, error) {
	request := &SearchRequest{Query: query, Options: NewSearchOptions()}
	if err := request.Validate(); err != nil {
		return nil, fmt.Errorf("vectorstore: create search request: %w", err)
	}
	return request, nil
}

func (s *SearchRequest) Validate() error {
	if s == nil {
		return fmt.Errorf("%w: search request is nil", ErrInvalidRequest)
	}
	if strings.TrimSpace(s.Query) == "" {
		return fmt.Errorf("%w: query must not be empty", ErrInvalidRequest)
	}
	if err := s.Options.Validate(); err != nil {
		return fmt.Errorf("%w: options: %w", ErrInvalidRequest, err)
	}
	return nil
}

func (s SearchRequest) MarshalJSON() ([]byte, error) {
	if err := (&s).Validate(); err != nil {
		return nil, err
	}
	type wireSearchRequest SearchRequest
	return json.Marshal(wireSearchRequest(s))
}

func (s *SearchRequest) UnmarshalJSON(data []byte) error {
	if s == nil {
		return fmt.Errorf("%w: search request receiver is nil", ErrInvalidRequest)
	}
	type wireSearchRequest SearchRequest
	var decoded wireSearchRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("%w: decode search request: %w", ErrInvalidRequest, err)
	}
	candidate := SearchRequest(decoded)
	if err := candidate.Validate(); err != nil {
		return err
	}
	*s = candidate
	return nil
}

// SearchResult relates a document to one search operation. Score is deliberately
// kept outside document.Document: relevance belongs to a query/result pair,
// not to the indexed content itself.
type SearchResult struct {
	Document *document.Document `json:"document"`
	Score    Score              `json:"score"`
}

func NewSearchResult(document *document.Document, score Score) (*SearchResult, error) {
	result := &SearchResult{Document: document, Score: score}
	if err := result.Validate(); err != nil {
		return nil, fmt.Errorf("vectorstore: create search result: %w", err)
	}
	return result, nil
}

func (s *SearchResult) Validate() error {
	if s == nil {
		return fmt.Errorf("%w: result is nil", ErrInvalidResponse)
	}
	if s.Document == nil {
		return fmt.Errorf("%w: result document is nil", ErrInvalidResponse)
	}
	if err := s.Document.Validate(); err != nil {
		return fmt.Errorf("%w: result document: %w", ErrInvalidResponse, err)
	}
	if strings.TrimSpace(s.Document.ID) == "" {
		return fmt.Errorf("%w: result: %w", ErrInvalidResponse, ErrMissingDocumentID)
	}
	if err := s.Score.Validate(); err != nil {
		return fmt.Errorf("%w: result score: %w", ErrInvalidResponse, err)
	}
	return nil
}

func (s SearchResult) MarshalJSON() ([]byte, error) {
	if err := (&s).Validate(); err != nil {
		return nil, err
	}
	type wireSearchResult SearchResult
	return json.Marshal(wireSearchResult(s))
}

func (s *SearchResult) UnmarshalJSON(data []byte) error {
	if s == nil {
		return fmt.Errorf("%w: search result receiver is nil", ErrInvalidResponse)
	}
	type wireSearchResult SearchResult
	var decoded wireSearchResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("%w: decode search result: %w", ErrInvalidResponse, err)
	}
	candidate := SearchResult(decoded)
	if err := candidate.Validate(); err != nil {
		return err
	}
	*s = candidate
	return nil
}

// SearchResponse owns a complete ranked result set.
type SearchResponse struct {
	Results []*SearchResult `json:"results"`
}

func NewSearchResponse(results []*SearchResult) (*SearchResponse, error) {
	response := &SearchResponse{Results: slices.Clone(results)}
	if err := response.Validate(); err != nil {
		return nil, fmt.Errorf("vectorstore: create search response: %w", err)
	}
	return response, nil
}

func (s *SearchResponse) Validate() error {
	if s == nil {
		return fmt.Errorf("%w: response is nil", ErrInvalidResponse)
	}
	for index, result := range s.Results {
		if err := result.Validate(); err != nil {
			return fmt.Errorf("%w: results[%d]: %w", ErrInvalidResponse, index, err)
		}
		if index > 0 && s.Results[index-1].Score < result.Score {
			return fmt.Errorf("%w: results are not sorted by descending score at index %d", ErrInvalidResponse, index)
		}
	}
	return nil
}

func (s SearchResponse) MarshalJSON() ([]byte, error) {
	if err := (&s).Validate(); err != nil {
		return nil, err
	}
	type wireSearchResponse SearchResponse
	return json.Marshal(wireSearchResponse(s))
}

func (s *SearchResponse) UnmarshalJSON(data []byte) error {
	if s == nil {
		return fmt.Errorf("%w: search response receiver is nil", ErrInvalidResponse)
	}
	type wireSearchResponse SearchResponse
	var decoded wireSearchResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("%w: decode search response: %w", ErrInvalidResponse, err)
	}
	candidate := SearchResponse(decoded)
	if err := candidate.Validate(); err != nil {
		return err
	}
	*s = candidate
	return nil
}

func (s *SearchResponse) ValidateFor(request *SearchRequest) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if err := s.Validate(); err != nil {
		return err
	}
	if len(s.Results) > request.Options.TopK {
		return fmt.Errorf("%w: got %d results, top K is %d", ErrInvalidResponse, len(s.Results), request.Options.TopK)
	}
	for index, result := range s.Results {
		if result.Score < request.Options.MinScore {
			return fmt.Errorf("%w: results[%d] score %v is below minimum score %v",
				ErrInvalidResponse, index, result.Score, request.Options.MinScore)
		}
	}
	return nil
}

func (s *SearchResponse) First() *SearchResult {
	if s == nil || len(s.Results) == 0 {
		return nil
	}
	return s.Results[0]
}

func (s *SearchResponse) Documents() []*document.Document {
	if s == nil {
		return nil
	}
	documents := make([]*document.Document, len(s.Results))
	for i, result := range s.Results {
		if result != nil {
			documents[i] = result.Document
		}
	}
	return documents
}

// Searcher pulls documents similar to a query out of a vector store.
// Results are ranked by similarity score in descending order.
type Searcher interface {
	// Search returns a response honoring the score threshold, metadata filter,
	// and result cap owned by [SearchRequest.Options].
	Search(ctx context.Context, request *SearchRequest) (*SearchResponse, error)
}
