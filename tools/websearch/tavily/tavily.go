package tavily

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-resty/resty/v2"

	"github.com/Tangerg/lynx/tools/websearch"
)

const (
	Name    = "tavily"
	baseURL = "https://api.tavily.com"
)

// Config configures [NewClient].
type Config struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

// Client implements [websearch.Provider] against Tavily.
type Client struct {
	http *resty.Client
}

var _ websearch.Provider = (*Client)(nil)

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

func (c *Client) Name() string { return Name }

type request struct {
	Query          string   `json:"query"`
	SearchDepth    string   `json:"search_depth,omitempty"`
	Topic          string   `json:"topic,omitempty"`
	MaxResults     int      `json:"max_results,omitempty"`
	TimeRange      string   `json:"time_range,omitempty"`
	IncludeDomains []string `json:"include_domains,omitempty"`
	ExcludeDomains []string `json:"exclude_domains,omitempty"`
	IncludeFavicon bool     `json:"include_favicon,omitempty"`
}

func (r *request) validate() error {
	if r == nil {
		return errors.New("tavily: Request must not be nil")
	}
	if r.Query == "" {
		return errors.New("tavily: Query must not be empty")
	}
	return nil
}

type result struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
	Favicon string `json:"favicon,omitempty"`
}

type response struct {
	Query   string    `json:"query"`
	Results []*result `json:"results"`
}

func (c *Client) search(ctx context.Context, req *request) (*response, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}
	var raw response
	resp, err := c.http.R().SetContext(ctx).SetBody(req).SetResult(&raw).Post("/search")
	if err != nil {
		return nil, fmt.Errorf("tavily: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("tavily: API error (status %d): %s", resp.StatusCode(), resp.String())
	}
	return &raw, nil
}

func (c *Client) Search(ctx context.Context, req *websearch.Request) (*websearch.Response, error) {
	raw, err := c.search(ctx, buildRequest(req))
	if err != nil {
		return nil, err
	}
	return raw.toWebSearch(), nil
}

const maxResultsCap = 20

func buildRequest(req *websearch.Request) *request {
	r := &request{
		Query:          req.Query,
		SearchDepth:    "basic",
		Topic:          "general",
		MaxResults:     clampResults(req.MaxResults),
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

// clampResults applies Tavily's [1, 20] bound with a 5 default.
func clampResults(n int) int {
	return min(cmp.Or(n, 5), maxResultsCap)
}

func recencyToTimeRange(r websearch.Recency) string {
	switch r {
	case websearch.RecencyHour, websearch.RecencyDay:
		return "day" // Tavily's minimum granularity
	case websearch.RecencyWeek:
		return "week"
	case websearch.RecencyMonth:
		return "month"
	case websearch.RecencyYear:
		return "year"
	}
	return ""
}

func (r *response) toWebSearch() *websearch.Response {
	results := make([]*websearch.Result, 0, len(r.Results))
	for _, result := range r.Results {
		results = append(results, &websearch.Result{
			Title:      result.Title,
			URL:        result.URL,
			Snippet:    result.Content,
			FaviconURL: result.Favicon,
		})
	}
	return &websearch.Response{Query: r.Query, Results: results}
}
