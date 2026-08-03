package firecrawl

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
	Name    = "firecrawl"
	baseURL = "https://api.firecrawl.dev/v2"
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

// NewClient returns a Firecrawl-backed client.
func NewClient(cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("firecrawl: APIKey is required")
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
	Query string `json:"query"`
	Limit int    `json:"limit,omitempty"`
	Tbs   string `json:"tbs,omitempty"`
}

func (r *request) validate() error {
	if r == nil {
		return errors.New("firecrawl: Request must not be nil")
	}
	if r.Query == "" {
		return errors.New("firecrawl: Query must not be empty")
	}
	return nil
}

type result struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

type responseData struct {
	Web []*result `json:"web,omitempty"`
}

type response struct {
	Success bool         `json:"success"`
	Data    responseData `json:"data"`
}

func (c *Client) search(ctx context.Context, req *request) (*response, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}
	var raw response
	resp, err := c.http.R().SetContext(ctx).SetBody(req).SetResult(&raw).Post("/search")
	if err != nil {
		return nil, fmt.Errorf("firecrawl: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("firecrawl: API error (status %d): %s", resp.StatusCode(), resp.String())
	}
	if !raw.Success {
		return nil, fmt.Errorf("firecrawl: search failed: %s", resp.String())
	}
	return &raw, nil
}

func (c *Client) Search(ctx context.Context, req *websearch.Request) (*websearch.Response, error) {
	raw, err := c.search(ctx, buildRequest(req))
	if err != nil {
		return nil, err
	}
	return raw.toWebSearch(req.Query), nil
}

const maxResultsCap = 100

func buildRequest(req *websearch.Request) *request {
	r := &request{
		Query: websearch.BuildSiteOperatorQuery(req.Query, req.AllowedDomains, req.BlockedDomains),
		Limit: min(cmp.Or(req.MaxResults, 10), maxResultsCap),
	}
	r.Tbs = recencyToTbs(req.Recency)
	return r
}

func recencyToTbs(r websearch.Recency) string {
	switch r {
	case websearch.RecencyHour:
		return "qdr:h"
	case websearch.RecencyDay:
		return "qdr:d"
	case websearch.RecencyWeek:
		return "qdr:w"
	case websearch.RecencyMonth:
		return "qdr:m"
	case websearch.RecencyYear:
		return "qdr:y"
	}
	return ""
}

func (r *response) toWebSearch(query string) *websearch.Response {
	results := make([]*websearch.Result, 0, len(r.Data.Web))
	for _, result := range r.Data.Web {
		results = append(results, &websearch.Result{
			Title:   result.Title,
			URL:     result.URL,
			Snippet: result.Description,
		})
	}
	return &websearch.Response{Query: query, Results: results}
}
