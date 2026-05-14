package perplexity

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/Tangerg/lynx/tools/websearch"
)

const (
	Name    = "perplexity"
	baseURL = "https://api.perplexity.ai"
)

type Config struct {
	APIKey string
}

func (c *Config) Validate() error {
	if c == nil {
		return errors.New("perplexity: Config must not be nil")
	}
	if c.APIKey == "" {
		return errors.New("perplexity: APIKey is required")
	}
	return nil
}

type Client struct {
	http *resty.Client
}

var _ websearch.Provider = (*Client)(nil)

func NewClient(cfg *Config) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Client{
		http: resty.New().
			SetBaseURL(baseURL).
			SetAuthToken(cfg.APIKey).
			SetHeader("Content-Type", "application/json"),
	}, nil
}

func (c *Client) Name() string { return Name }

// ============================================================== Native API

// Request is the full Perplexity /search request shape.
type Request struct {
	// Query is the search string OR a []string of multiple queries.
	// Use any to support both forms.
	Query any `json:"query"`

	// MaxTokens caps the LLM context budget (default 10000, range 1-1M).
	MaxTokens int `json:"max_tokens,omitempty"`

	// MaxTokensPerPage caps per-page tokens (default 4096).
	MaxTokensPerPage int `json:"max_tokens_per_page,omitempty"`

	// MaxResults caps result count (default 10, range 1-20).
	MaxResults int `json:"max_results,omitempty"`

	// SearchDomainFilter holds at most 20 entries. Prefix a domain
	// with "-" to blacklist it; mixing allow/block in one call is not
	// supported by the API.
	SearchDomainFilter []string `json:"search_domain_filter,omitempty"`

	// SearchLanguageFilter is a list of ISO 639-1 codes (max 20).
	SearchLanguageFilter []string `json:"search_language_filter,omitempty"`

	// SearchRecencyFilter: hour, day, week, month, year.
	SearchRecencyFilter string `json:"search_recency_filter,omitempty"`

	// SearchAfterDateFilter / SearchBeforeDateFilter use MM/DD/YYYY.
	SearchAfterDateFilter  string `json:"search_after_date_filter,omitempty"`
	SearchBeforeDateFilter string `json:"search_before_date_filter,omitempty"`

	// LastUpdatedAfterFilter / LastUpdatedBeforeFilter use MM/DD/YYYY.
	LastUpdatedAfterFilter  string `json:"last_updated_after_filter,omitempty"`
	LastUpdatedBeforeFilter string `json:"last_updated_before_filter,omitempty"`

	// Country is an ISO 3166-1 alpha-2 code.
	Country string `json:"country,omitempty"`
}

// Validate enforces request invariants. Perplexity's Query field
// accepts string or []string; both must be non-empty.
func (r *Request) Validate() error {
	if r == nil {
		return errors.New("perplexity: Request must not be nil")
	}
	switch q := r.Query.(type) {
	case nil:
		return errors.New("perplexity: Query is required")
	case string:
		if q == "" {
			return errors.New("perplexity: Query must not be empty")
		}
	case []string:
		if len(q) == 0 {
			return errors.New("perplexity: Query must not be empty")
		}
	default:
		return fmt.Errorf("perplexity: Query must be string or []string, got %T", r.Query)
	}
	return nil
}

// Result is one item in [Response.Results].
type Result struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Snippet     string `json:"snippet"`
	Date        string `json:"date,omitempty"`
	LastUpdated string `json:"last_updated,omitempty"`
}

// Response is the full Perplexity /search response.
type Response struct {
	Results    []*Result `json:"results"`
	ID         string    `json:"id"`
	ServerTime string    `json:"server_time,omitempty"`
}

// SearchNative calls POST /search with the full Perplexity request.
func (c *Client) SearchNative(ctx context.Context, req *Request) (*Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	var raw Response
	resp, err := c.http.R().SetContext(ctx).SetBody(req).SetResult(&raw).Post("/search")
	if err != nil {
		return nil, fmt.Errorf("perplexity: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("perplexity: API error (status %d): %s", resp.StatusCode(), resp.String())
	}
	return &raw, nil
}

// ============================================================== SPI wrapper

func (c *Client) Search(ctx context.Context, req *websearch.Request) (*websearch.Response, error) {
	raw, err := c.SearchNative(ctx, buildRequest(req))
	if err != nil {
		return nil, err
	}
	return shapeResponse(req.Query, raw), nil
}

// ============================================================== mapping

// maxDomainFilters is Perplexity's documented 20-entry cap on the
// search_domain_filter field.
const maxDomainFilters = 20

// maxResultsCap matches Perplexity's documented upper bound; the API
// rejects values outside [1, 20].
const maxResultsCap = 20

func buildRequest(req *websearch.Request) *Request {
	r := &Request{Query: req.Query}
	if req.MaxResults > 0 {
		r.MaxResults = min(req.MaxResults, maxResultsCap)
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
// downstream (the negation loop in buildRequest writes in place).
func capDomains(in []string) []string {
	return slices.Clone(in[:min(len(in), maxDomainFilters)])
}

func recencyToString(r websearch.Recency) string {
	switch r {
	case websearch.RecencyHour, websearch.RecencyDay, websearch.RecencyWeek, websearch.RecencyMonth, websearch.RecencyYear:
		return string(r)
	}
	return ""
}

func shapeResponse(query string, raw *Response) *websearch.Response {
	results := make([]*websearch.Result, 0, len(raw.Results))
	for _, r := range raw.Results {
		results = append(results, &websearch.Result{
			Title:         r.Title,
			URL:           r.URL,
			Snippet:       r.Snippet,
			PublishedTime: parseDate(r.Date),
		})
	}
	// Perplexity doesn't echo the query; pass through the requester's.
	return &websearch.Response{Query: query, Results: results}
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
