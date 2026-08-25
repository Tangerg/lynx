package exa

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/Tangerg/lynx/tools/websearch"
)

const (
	Name    = "exa"
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

var _ websearch.Provider = (*Client)(nil)

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

func (c *Client) Name() string { return Name }

type summaryOptions struct {
	Query string `json:"query,omitempty"`
}

type contentsOptions struct {
	Summary *summaryOptions `json:"summary,omitempty"`
}

type request struct {
	Query              string           `json:"query"`
	Type               string           `json:"type,omitempty"`
	NumResults         int              `json:"numResults,omitempty"`
	IncludeDomains     []string         `json:"includeDomains,omitempty"`
	ExcludeDomains     []string         `json:"excludeDomains,omitempty"`
	StartPublishedDate string           `json:"startPublishedDate,omitempty"`
	Contents           *contentsOptions `json:"contents,omitempty"`
}

func (r *request) validate() error {
	if r == nil {
		return errors.New("exa: Request must not be nil")
	}
	if r.Query == "" {
		return errors.New("exa: Query must not be empty")
	}
	return nil
}

type result struct {
	Title         string   `json:"title"`
	URL           string   `json:"url"`
	PublishedDate string   `json:"publishedDate,omitempty"`
	Author        string   `json:"author,omitempty"`
	Favicon       string   `json:"favicon,omitempty"`
	Highlights    []string `json:"highlights,omitempty"`
	Summary       string   `json:"summary,omitempty"`
}

type response struct {
	Results []*result `json:"results"`
}

func (c *Client) search(ctx context.Context, req *request) (*response, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}
	var raw response
	resp, err := c.http.R().SetContext(ctx).SetBody(req).SetResult(&raw).Post("/search")
	if err != nil {
		return nil, fmt.Errorf("exa: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("exa: API error (status %d): %s", resp.StatusCode(), resp.String())
	}
	return &raw, nil
}

func (c *Client) Search(ctx context.Context, req *websearch.Request) (*websearch.Response, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("exa: %w", err)
	}
	raw, err := c.search(ctx, buildRequest(req))
	if err != nil {
		return nil, err
	}
	return raw.toWebSearch(req.Query), nil
}

func buildRequest(req *websearch.Request) *request {
	r := &request{
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

func recencyToStart(r websearch.Recency) time.Time {
	now := time.Now()
	switch r {
	case websearch.RecencyHour:
		return now.Add(-time.Hour)
	case websearch.RecencyDay:
		return now.Add(-24 * time.Hour)
	case websearch.RecencyWeek:
		return now.Add(-7 * 24 * time.Hour)
	case websearch.RecencyMonth:
		return now.AddDate(0, -1, 0)
	case websearch.RecencyYear:
		return now.AddDate(-1, 0, 0)
	}
	return time.Time{}
}

func (r *response) toWebSearch(query string) *websearch.Response {
	results := make([]*websearch.Result, 0, len(r.Results))
	for _, result := range r.Results {
		results = append(results, &websearch.Result{
			Title:         result.Title,
			URL:           result.URL,
			Snippet:       result.snippet(),
			FaviconURL:    result.Favicon,
			Source:        result.Author,
			PublishedTime: parseDate(result.PublishedDate),
		})
	}
	return &websearch.Response{Query: query, Results: results}
}

func (r *result) snippet() string {
	if len(r.Highlights) > 0 {
		return r.Highlights[0]
	}
	return r.Summary
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
