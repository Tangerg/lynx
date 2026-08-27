package firecrawl

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

var _ web.Searcher = (*Client)(nil)

// NewClient returns a Firecrawl-backed client.
func NewClient(config Config) (*Client, error) {
	if config.APIKey == "" {
		return nil, errors.New("firecrawl: APIKey is required")
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
	Query string `json:"query"`
	Limit int    `json:"limit,omitempty"`
	Tbs   string `json:"tbs,omitempty"`
}

func (s *searchRequest) validate() error {
	if s == nil {
		return errors.New("firecrawl: Request must not be nil")
	}
	if s.Query == "" {
		return errors.New("firecrawl: Query must not be empty")
	}
	return nil
}

type searchResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

type searchResponseData struct {
	Web []*searchResult `json:"web,omitempty"`
}

type searchResponse struct {
	Success bool               `json:"success"`
	Data    searchResponseData `json:"data"`
}

func (c *Client) search(ctx context.Context, req *searchRequest) (*searchResponse, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}
	var raw searchResponse
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

func (c *Client) Search(ctx context.Context, req *web.SearchRequest) (*web.SearchResponse, error) {
	prepared, err := req.Prepare()
	if err != nil {
		return nil, fmt.Errorf("firecrawl: %w", err)
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
		Query: req.QueryWithSiteOperators(),
		Limit: cmp.Or(req.MaxResults, 10),
	}
	r.Tbs = recencyToTbs(req.Recency)
	return r
}

func recencyToTbs(r web.Recency) string {
	switch r {
	case web.RecencyHour:
		return "qdr:h"
	case web.RecencyDay:
		return "qdr:d"
	case web.RecencyWeek:
		return "qdr:w"
	case web.RecencyMonth:
		return "qdr:m"
	case web.RecencyYear:
		return "qdr:y"
	}
	return ""
}

func (s *searchResponse) toSearchResponse(query string) *web.SearchResponse {
	results := make([]*web.SearchResult, 0, len(s.Data.Web))
	for _, searchResult := range s.Data.Web {
		results = append(results, &web.SearchResult{
			Title:   searchResult.Title,
			URL:     searchResult.URL,
			Snippet: searchResult.Description,
		})
	}
	return &web.SearchResponse{Query: query, Results: results}
}
