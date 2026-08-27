package tavily

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-resty/resty/v2"

	"github.com/Tangerg/scope/tools/web"
)

const (
	baseURL = "https://api.tavily.com"
)

// Config configures [NewClient].
type Config struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

// Client implements [web.Searcher] against Tavily.
type Client struct {
	http *resty.Client
}

var _ web.Searcher = (*Client)(nil)

// NewClient returns a Tavily-backed client.
func NewClient(cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("tavily: APIKey is required")
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = baseURL
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{}
	}
	return &Client{
		http: resty.NewWithClient(cfg.HTTPClient).
			SetBaseURL(cfg.BaseURL).
			SetAuthToken(cfg.APIKey).
			SetHeader("Content-Type", "application/json"),
	}, nil
}

type searchRequest struct {
	Query          string   `json:"query"`
	SearchDepth    string   `json:"search_depth,omitempty"`
	Topic          string   `json:"topic,omitempty"`
	MaxResults     int      `json:"max_results,omitempty"`
	TimeRange      string   `json:"time_range,omitempty"`
	IncludeDomains []string `json:"include_domains,omitempty"`
	ExcludeDomains []string `json:"exclude_domains,omitempty"`
	IncludeFavicon bool     `json:"include_favicon,omitempty"`
}

func (s *searchRequest) validate() error {
	if s == nil {
		return errors.New("tavily: Request must not be nil")
	}
	if s.Query == "" {
		return errors.New("tavily: Query must not be empty")
	}
	return nil
}

type searchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
	Favicon string `json:"favicon,omitempty"`
}

type searchResponse struct {
	Query   string          `json:"query"`
	Results []*searchResult `json:"results"`
}

func (c *Client) search(ctx context.Context, req *searchRequest) (*searchResponse, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}
	var raw searchResponse
	resp, err := c.http.R().SetContext(ctx).SetBody(req).SetResult(&raw).Post("/search")
	if err != nil {
		return nil, fmt.Errorf("tavily: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("tavily: API error (status %d): %s", resp.StatusCode(), resp.String())
	}
	return &raw, nil
}

func (c *Client) Search(ctx context.Context, req *web.SearchRequest) (*web.SearchResponse, error) {
	prepared, err := req.Prepare()
	if err != nil {
		return nil, fmt.Errorf("tavily: %w", err)
	}
	req = prepared
	raw, err := c.search(ctx, buildSearchRequest(req))
	if err != nil {
		return nil, err
	}
	return raw.toSearchResponse(), nil
}

func buildSearchRequest(req *web.SearchRequest) *searchRequest {
	r := &searchRequest{
		Query:          req.Query,
		SearchDepth:    "basic",
		Topic:          "general",
		MaxResults:     cmp.Or(req.MaxResults, 5),
		IncludeFavicon: true,
	}
	if len(req.AllowedDomains) > 0 {
		r.IncludeDomains = req.AllowedDomains
	}
	if len(req.BlockedDomains) > 0 {
		r.ExcludeDomains = req.BlockedDomains
	}
	r.TimeRange = recencyToTimeRange(req.Recency)
	return r
}

func recencyToTimeRange(r web.Recency) string {
	switch r {
	case web.RecencyHour, web.RecencyDay:
		return "day" // Tavily's minimum granularity
	case web.RecencyWeek:
		return "week"
	case web.RecencyMonth:
		return "month"
	case web.RecencyYear:
		return "year"
	}
	return ""
}

func (s *searchResponse) toSearchResponse() *web.SearchResponse {
	results := make([]*web.SearchResult, 0, len(s.Results))
	for _, searchResult := range s.Results {
		results = append(results, &web.SearchResult{
			Title:      searchResult.Title,
			URL:        searchResult.URL,
			Snippet:    searchResult.Content,
			FaviconURL: searchResult.Favicon,
		})
	}
	return &web.SearchResponse{Query: s.Query, Results: results}
}
