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

const defaultSearchResultCount = 10

const (
	baseURL = "https://api.firecrawl.dev/v2"
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
		return nil, errors.New("firecrawl: API key is required")
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
		return errors.New("firecrawl: search request must not be nil")
	}
	if s.Query == "" {
		return errors.New("firecrawl: search query must not be empty")
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

func (c *Client) search(ctx context.Context, request *searchRequest) (*searchResponse, error) {
	if err := request.validate(); err != nil {
		return nil, err
	}
	var raw searchResponse
	response, err := c.http.R().SetContext(ctx).SetBody(request).SetResult(&raw).Post("/search")
	if err != nil {
		return nil, fmt.Errorf("firecrawl: execute search request: %w", err)
	}
	if !response.IsSuccess() {
		return nil, fmt.Errorf("firecrawl: search request returned HTTP %d: %s", response.StatusCode(), response.String())
	}
	if !raw.Success {
		return nil, fmt.Errorf("firecrawl: search response reported failure: %s", response.String())
	}
	return &raw, nil
}

func (c *Client) Search(ctx context.Context, request *web.SearchRequest) (*web.SearchResponse, error) {
	prepared, err := request.Prepare()
	if err != nil {
		return nil, fmt.Errorf("firecrawl: prepare search request: %w", err)
	}
	request = prepared
	raw, err := c.search(ctx, buildSearchRequest(request))
	if err != nil {
		return nil, err
	}
	return raw.toSearchResponse(request.Query), nil
}

func buildSearchRequest(request *web.SearchRequest) *searchRequest {
	r := &searchRequest{
		Query: request.QueryWithSiteOperators(),
		Limit: cmp.Or(request.MaxResults, defaultSearchResultCount),
	}
	r.Tbs = recencyToTbs(request.Recency)
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
