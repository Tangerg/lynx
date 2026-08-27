package exa

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/Tangerg/lynx/tools/web"
)

const (
	baseURL = "https://api.exa.ai"
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

// NewClient returns an Exa-backed client.
func NewClient(cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("exa: APIKey is required")
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
			SetHeader("x-api-key", cfg.APIKey).
			SetHeader("Content-Type", "application/json"),
	}, nil
}

type summaryOptions struct {
	Query string `json:"query,omitempty"`
}

type contentsOptions struct {
	Summary *summaryOptions `json:"summary,omitempty"`
}

type searchRequest struct {
	Query              string           `json:"query"`
	Type               string           `json:"type,omitempty"`
	NumResults         int              `json:"numResults,omitempty"`
	IncludeDomains     []string         `json:"includeDomains,omitempty"`
	ExcludeDomains     []string         `json:"excludeDomains,omitempty"`
	StartPublishedDate string           `json:"startPublishedDate,omitempty"`
	Contents           *contentsOptions `json:"contents,omitempty"`
}

func (s *searchRequest) validate() error {
	if s == nil {
		return errors.New("exa: Request must not be nil")
	}
	if s.Query == "" {
		return errors.New("exa: Query must not be empty")
	}
	return nil
}

type searchResult struct {
	Title         string   `json:"title"`
	URL           string   `json:"url"`
	PublishedDate string   `json:"publishedDate,omitempty"`
	Author        string   `json:"author,omitempty"`
	Favicon       string   `json:"favicon,omitempty"`
	Highlights    []string `json:"highlights,omitempty"`
	Summary       string   `json:"summary,omitempty"`
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
		return nil, fmt.Errorf("exa: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("exa: API error (status %d): %s", resp.StatusCode(), resp.String())
	}
	return &raw, nil
}

func (c *Client) Search(ctx context.Context, req *web.SearchRequest) (*web.SearchResponse, error) {
	prepared, err := req.Prepare()
	if err != nil {
		return nil, fmt.Errorf("exa: %w", err)
	}
	req = prepared
	raw, err := c.search(ctx, buildSearchRequest(req))
	if err != nil {
		return nil, err
	}
	return raw.toSearchResponse(req.Query), nil
}

func buildSearchRequest(req *web.SearchRequest) *searchRequest {
	r := &searchRequest{
		Query:      req.Query,
		Type:       "fast",
		NumResults: cmp.Or(req.MaxResults, 10),
		Contents: &contentsOptions{
			Summary: &summaryOptions{Query: req.Query},
		},
	}
	if len(req.AllowedDomains) > 0 {
		r.IncludeDomains = req.AllowedDomains
	}
	if len(req.BlockedDomains) > 0 {
		r.ExcludeDomains = req.BlockedDomains
	}
	if start := recencyToStart(req.Recency); !start.IsZero() {
		r.StartPublishedDate = start.Format(time.RFC3339)
	}
	return r
}

func recencyToStart(r web.Recency) time.Time {
	now := time.Now()
	switch r {
	case web.RecencyHour:
		return now.Add(-time.Hour)
	case web.RecencyDay:
		return now.Add(-24 * time.Hour)
	case web.RecencyWeek:
		return now.Add(-7 * 24 * time.Hour)
	case web.RecencyMonth:
		return now.AddDate(0, -1, 0)
	case web.RecencyYear:
		return now.AddDate(-1, 0, 0)
	}
	return time.Time{}
}

func (s *searchResponse) toSearchResponse(query string) *web.SearchResponse {
	results := make([]*web.SearchResult, 0, len(s.Results))
	for _, searchResult := range s.Results {
		results = append(results, &web.SearchResult{
			Title:         searchResult.Title,
			URL:           searchResult.URL,
			Snippet:       searchResult.snippet(),
			FaviconURL:    searchResult.Favicon,
			Source:        searchResult.Author,
			PublishedTime: parseDate(searchResult.PublishedDate),
		})
	}
	return &web.SearchResponse{Query: query, Results: results}
}

func (s *searchResult) snippet() string {
	if len(s.Highlights) > 0 {
		return s.Highlights[0]
	}
	return s.Summary
}

func parseDate(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return time.Time{}
}
