package vectorstore

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/vectorstore/filter"
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

// NewSearchOptions returns the recommended provider-neutral defaults.
func NewSearchOptions() SearchOptions {
	return SearchOptions{TopK: DefaultTopK}
}

// Validate verifies every search policy independently from its query.
func (o SearchOptions) Validate() error {
	if o.TopK <= 0 {
		return fmt.Errorf("%w: TopK must be > 0, got %d", ErrInvalidOptions, o.TopK)
	}
	if err := o.MinScore.Validate(); err != nil {
		return fmt.Errorf("%w: MinScore: %w", ErrInvalidOptions, err)
	}
	if o.Filter != nil {
		if err := o.Filter.Validate(); err != nil {
			return fmt.Errorf("%w: filter: %w", ErrInvalidOptions, err)
		}
	}
	return nil
}

func (o SearchOptions) MarshalJSON() ([]byte, error) {
	if err := o.Validate(); err != nil {
		return nil, err
	}
	type wireSearchOptions SearchOptions
	return json.Marshal(wireSearchOptions(o))
}

func (o *SearchOptions) UnmarshalJSON(data []byte) error {
	if o == nil {
		return fmt.Errorf("%w: nil SearchOptions receiver", ErrInvalidOptions)
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
	*o = candidate
	return nil
}

// SearchRequest describes one semantic search and owns its input validation.
type SearchRequest struct {
	Query   string        `json:"query,omitempty"`
	Options SearchOptions `json:"options"`
}

// NewSearchRequest creates a request with the recommended search options.
func NewSearchRequest(query string) (*SearchRequest, error) {
	request := &SearchRequest{Query: query, Options: NewSearchOptions()}
	if err := request.Validate(); err != nil {
		return nil, fmt.Errorf("vectorstore.NewSearchRequest: %w", err)
	}
	return request, nil
}

// Validate verifies the query and its options before provider I/O.
func (r *SearchRequest) Validate() error {
	if r == nil {
		return fmt.Errorf("%w: search request is nil", ErrInvalidRequest)
	}
	if strings.TrimSpace(r.Query) == "" {
		return fmt.Errorf("%w: Query must not be empty", ErrInvalidRequest)
	}
	if err := r.Options.Validate(); err != nil {
		return fmt.Errorf("%w: options: %w", ErrInvalidRequest, err)
	}
	return nil
}

func (r SearchRequest) MarshalJSON() ([]byte, error) {
	if err := (&r).Validate(); err != nil {
		return nil, err
	}
	type wireSearchRequest SearchRequest
	return json.Marshal(wireSearchRequest(r))
}

func (r *SearchRequest) UnmarshalJSON(data []byte) error {
	if r == nil {
		return fmt.Errorf("%w: nil SearchRequest receiver", ErrInvalidRequest)
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
	*r = candidate
	return nil
}

// SearchResult relates a document to one search operation. Score is deliberately
// kept outside document.Document: relevance belongs to a query/result pair,
// not to the indexed content itself.
type SearchResult struct {
	Document *document.Document `json:"document"`
	Score    Score              `json:"score"`
}

// NewSearchResult creates and validates one ranked document result.
func NewSearchResult(doc *document.Document, score Score) (*SearchResult, error) {
	result := &SearchResult{Document: doc, Score: score}
	if err := result.Validate(); err != nil {
		return nil, fmt.Errorf("vectorstore.NewSearchResult: %w", err)
	}
	return result, nil
}

func (r *SearchResult) Validate() error {
	if r == nil {
		return fmt.Errorf("%w: result is nil", ErrInvalidResponse)
	}
	if r.Document == nil {
		return fmt.Errorf("%w: result document is nil", ErrInvalidResponse)
	}
	if err := r.Document.Validate(); err != nil {
		return fmt.Errorf("%w: result document: %w", ErrInvalidResponse, err)
	}
	if strings.TrimSpace(r.Document.ID) == "" {
		return fmt.Errorf("%w: result: %w", ErrInvalidResponse, ErrMissingDocumentID)
	}
	if err := r.Score.Validate(); err != nil {
		return fmt.Errorf("%w: result score: %w", ErrInvalidResponse, err)
	}
	return nil
}

func (r SearchResult) MarshalJSON() ([]byte, error) {
	if err := (&r).Validate(); err != nil {
		return nil, err
	}
	type wireSearchResult SearchResult
	return json.Marshal(wireSearchResult(r))
}

func (r *SearchResult) UnmarshalJSON(data []byte) error {
	if r == nil {
		return fmt.Errorf("%w: nil SearchResult receiver", ErrInvalidResponse)
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
	*r = candidate
	return nil
}

// SearchResponse owns a complete ranked result set.
type SearchResponse struct {
	Results []*SearchResult `json:"results"`
}

// NewSearchResponse creates and validates a complete ranked result set.
func NewSearchResponse(results []*SearchResult) (*SearchResponse, error) {
	response := &SearchResponse{Results: slices.Clone(results)}
	if err := response.Validate(); err != nil {
		return nil, fmt.Errorf("vectorstore.NewSearchResponse: %w", err)
	}
	return response, nil
}

// Validate verifies the response's provider-independent invariants.
func (r *SearchResponse) Validate() error {
	if r == nil {
		return fmt.Errorf("%w: response is nil", ErrInvalidResponse)
	}
	for i, result := range r.Results {
		if err := result.Validate(); err != nil {
			return fmt.Errorf("%w: results[%d]: %w", ErrInvalidResponse, i, err)
		}
		if i > 0 && r.Results[i-1].Score < result.Score {
			return fmt.Errorf("%w: results are not sorted by descending score at index %d", ErrInvalidResponse, i)
		}
	}
	return nil
}

func (r SearchResponse) MarshalJSON() ([]byte, error) {
	if err := (&r).Validate(); err != nil {
		return nil, err
	}
	type wireSearchResponse SearchResponse
	return json.Marshal(wireSearchResponse(r))
}

func (r *SearchResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return fmt.Errorf("%w: nil SearchResponse receiver", ErrInvalidResponse)
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
	*r = candidate
	return nil
}

// ValidateFor verifies that a valid response also honors request policy.
func (r *SearchResponse) ValidateFor(request *SearchRequest) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if err := r.Validate(); err != nil {
		return err
	}
	if len(r.Results) > request.Options.TopK {
		return fmt.Errorf("%w: got %d results, TopK is %d", ErrInvalidResponse, len(r.Results), request.Options.TopK)
	}
	for i, result := range r.Results {
		if result.Score < request.Options.MinScore {
			return fmt.Errorf("%w: results[%d] score %v is below MinScore %v",
				ErrInvalidResponse, i, result.Score, request.Options.MinScore)
		}
	}
	return nil
}

// First returns the highest-ranked result, or nil when the response is empty.
func (r *SearchResponse) First() *SearchResult {
	if r == nil || len(r.Results) == 0 {
		return nil
	}
	return r.Results[0]
}

// Documents projects the ranked response into its complete documents.
func (r *SearchResponse) Documents() []*document.Document {
	if r == nil {
		return nil
	}
	documents := make([]*document.Document, len(r.Results))
	for i, result := range r.Results {
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
