package perplexity

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/Tangerg/scope/tools/web"
)

const (
	baseURL = "https://api.perplexity.ai"
)

// Config configures [NewClient].
type Config struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

type Client struct {
	http *resty.Client
}

var _ web.Searcher = (*Client)(nil)

// NewClient returns a Perplexity-backed client.
func NewClient(config Config) (*Client, error) {
	if config.APIKey == "" {
		return nil, errors.New("perplexity: APIKey is required")
	}
	if config.BaseURL == "" {
		config.BaseURL = baseURL
	}
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{}
	}
	return &Client{
		http: resty.NewWithClient(config.HTTPClient).
			SetBaseURL(config.BaseURL).
			SetAuthToken(config.APIKey).
			SetHeader("Content-Type", "application/json"),
	}, nil
}

type searchRequest struct {
	Query               string   `json:"query"`
	MaxResults          int      `json:"max_results,omitempty"`
	SearchDomainFilter  []string `json:"search_domain_filter,omitempty"`
	SearchRecencyFilter string   `json:"search_recency_filter,omitempty"`
}

func (s *searchRequest) validate() error {
	if s == nil {
		return errors.New("perplexity: Request must not be nil")
	}
	if s.Query == "" {
		return errors.New("perplexity: Query must not be empty")
	}
	return nil
}

type searchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
	Date    string `json:"date,omitempty"`
}

type searchResponse struct {
	Results []*searchResult `json:"results"`
}

func (c *Client) search(ctx context.Context, req *searchRequest) (*searchResponse, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}
	var raw searchResponse
	resp, err := c.http.R().SetContext(ctx).SetBody(req).SetResult(&raw).Post("/search")
	if err != nil {
		return nil, fmt.Errorf("perplexity: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("perplexity: API error (status %d): %s", resp.StatusCode(), resp.String())
	}
	return &raw, nil
}

func (c *Client) Search(ctx context.Context, req *web.SearchRequest) (*web.SearchResponse, error) {
	prepared, err := req.Prepare()
	if err != nil {
		return nil, fmt.Errorf("perplexity: %w", err)
	}
	req = prepared
	raw, err := c.search(ctx, buildSearchRequest(req))
	if err != nil {
		return nil, err
	}
	return raw.toSearchResponse(req.Query), nil
}

// maxDomainFilters is Perplexity's documented 20-entry cap on the
// search_domain_filter field.
const maxDomainFilters = 20

func buildSearchRequest(req *web.SearchRequest) *searchRequest {
	r := &searchRequest{Query: req.Query}
	if req.MaxResults > 0 {
		r.MaxResults = req.MaxResults
	}
	switch {
	case len(req.AllowedDomains) > 0:
		r.SearchDomainFilter = capDomains(req.AllowedDomains)
	case len(req.BlockedDomains) > 0:
		negated := capDomains(req.BlockedDomains)
		for i, d := range negated {
			negated[i] = "-" + d
		}
		r.SearchDomainFilter = negated
	}
	r.SearchRecencyFilter = recencyToString(req.Recency)
	return r
}

// capDomains caps the slice at Perplexity's documented 20-entry
// limit and returns a copy so the caller's slice isn't mutated
// downstream (the negation loop in buildSearchRequest writes in place).
func capDomains(in []string) []string {
	return slices.Clone(in[:min(len(in), maxDomainFilters)])
}

func recencyToString(r web.Recency) string {
	switch r {
	case web.RecencyHour, web.RecencyDay, web.RecencyWeek, web.RecencyMonth, web.RecencyYear:
		return string(r)
	}
	return ""
}

func (s *searchResponse) toSearchResponse(query string) *web.SearchResponse {
	results := make([]*web.SearchResult, 0, len(s.Results))
	for _, searchResult := range s.Results {
		results = append(results, &web.SearchResult{
			Title:         searchResult.Title,
			URL:           searchResult.URL,
			Snippet:       searchResult.Snippet,
			PublishedTime: parseDate(searchResult.Date),
		})
	}
	// Perplexity doesn't echo the query; pass through the requester's.
	return &web.SearchResponse{Query: query, Results: results}
}

func parseDate(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
