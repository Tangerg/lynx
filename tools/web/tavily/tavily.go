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

type Config struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

type Client struct {
	http *resty.Client
}

var _ web.Searcher = (*Client)(nil)

func NewClient(config Config) (*Client, error) {
	if config.APIKey == "" {
		return nil, errors.New("tavily: API key is required")
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
		return errors.New("tavily: search request must not be nil")
	}
	if s.Query == "" {
		return errors.New("tavily: search query must not be empty")
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

func (c *Client) search(ctx context.Context, request *searchRequest) (*searchResponse, error) {
	if err := request.validate(); err != nil {
		return nil, err
	}
	var raw searchResponse
	response, err := c.http.R().SetContext(ctx).SetBody(request).SetResult(&raw).Post("/search")
	if err != nil {
		return nil, fmt.Errorf("tavily: execute search request: %w", err)
	}
	if !response.IsSuccess() {
		return nil, fmt.Errorf("tavily: search request returned HTTP %d: %s", response.StatusCode(), response.String())
	}
	return &raw, nil
}

func (c *Client) Search(ctx context.Context, request *web.SearchRequest) (*web.SearchResponse, error) {
	prepared, err := request.Prepare()
	if err != nil {
		return nil, fmt.Errorf("tavily: prepare search request: %w", err)
	}
	request = prepared
	raw, err := c.search(ctx, buildSearchRequest(request))
	if err != nil {
		return nil, err
	}
	return raw.toSearchResponse(), nil
}

func buildSearchRequest(request *web.SearchRequest) *searchRequest {
	r := &searchRequest{
		Query:          request.Query,
		SearchDepth:    "basic",
		Topic:          "general",
		MaxResults:     cmp.Or(request.MaxResults, 5),
		IncludeFavicon: true,
	}
	if len(request.AllowedDomains) > 0 {
		r.IncludeDomains = request.AllowedDomains
	}
	if len(request.BlockedDomains) > 0 {
		r.ExcludeDomains = request.BlockedDomains
	}
	r.TimeRange = recencyToTimeRange(request.Recency)
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
