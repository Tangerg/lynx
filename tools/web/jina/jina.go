package jina

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/Tangerg/scope/tools/web"
)

const (
	searchBaseURL         = "https://s.jina.ai"
	fetchBaseURL          = "https://r.jina.ai"
	queryParameterCount   = "count"
	queryParameterPage    = "page"
	queryParameterSite    = "site"
	queryParameterNoCache = "noCache"
)

// Config configures [NewClient].
type Config struct {
	APIKey        string
	SearchBaseURL string
	FetchBaseURL  string
	HTTPClient    *http.Client
}

type Client struct {
	searchHTTP *resty.Client
	fetchHTTP  *resty.Client
}

var _ web.Searcher = (*Client)(nil)

// NewClient returns a Jina-backed search and page-fetching client.
func NewClient(cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("jina: APIKey is required")
	}
	if cfg.SearchBaseURL == "" {
		cfg.SearchBaseURL = searchBaseURL
	}
	if cfg.FetchBaseURL == "" {
		cfg.FetchBaseURL = fetchBaseURL
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{}
	}
	return &Client{
		searchHTTP: resty.NewWithClient(cfg.HTTPClient).
			SetBaseURL(cfg.SearchBaseURL).
			SetAuthToken(cfg.APIKey).
			SetHeader("Accept", "application/json").
			SetHeader("X-Respond-With", "no-content"),
		fetchHTTP: resty.NewWithClient(cfg.HTTPClient).
			SetBaseURL(cfg.FetchBaseURL).
			SetAuthToken(cfg.APIKey).
			SetHeader("Content-Type", "application/json").
			SetHeader("Accept", "application/json"),
	}, nil
}

type searchRequest struct {
	Query   string   `json:"-"`
	Count   int      `json:"count,omitempty"`
	Page    int      `json:"page,omitempty"`
	Site    []string `json:"site,omitempty"`
	NoCache bool     `json:"noCache,omitempty"`
}

func (s *searchRequest) validate() error {
	if s == nil {
		return errors.New("jina: Request must not be nil")
	}
	if s.Query == "" {
		return errors.New("jina: Query must not be empty")
	}
	return nil
}

type searchResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Content     string `json:"content,omitempty"`
	Date        string `json:"date,omitempty"`
}

type searchResponse struct {
	Data []*searchResult `json:"data"`
}

func (c *Client) search(ctx context.Context, req *searchRequest) (*searchResponse, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}
	endpoint := "/" + url.PathEscape(req.Query)
	params := req.params()

	var raw searchResponse
	resp, err := c.searchHTTP.R().SetContext(ctx).SetQueryParams(params).SetResult(&raw).Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("jina: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("jina: API error (status %d): %s", resp.StatusCode(), resp.String())
	}
	return &raw, nil
}

func (request *searchRequest) params() map[string]string {
	parameters := make(map[string]string, 4)
	if request.Count > 0 {
		parameters[queryParameterCount] = strconv.Itoa(request.Count)
	}
	if request.Page > 0 {
		parameters[queryParameterPage] = strconv.Itoa(request.Page)
	}
	if len(request.Site) > 0 {
		parameters[queryParameterSite] = strings.Join(request.Site, ",")
	}
	if request.NoCache {
		parameters[queryParameterNoCache] = strconv.FormatBool(request.NoCache)
	}
	return parameters
}

func (c *Client) Search(ctx context.Context, req *web.SearchRequest) (*web.SearchResponse, error) {
	prepared, err := req.Prepare()
	if err != nil {
		return nil, fmt.Errorf("jina: %w", err)
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
		Query: req.Query,
		Count: cmp.Or(req.MaxResults, 10),
		Page:  1,
	}
	if len(req.AllowedDomains) > 0 {
		r.Site = req.AllowedDomains
	}
	if req.Recency != "" {
		r.NoCache = true
	}
	return r
}

func (s *searchResponse) toSearchResponse(query string) *web.SearchResponse {
	results := make([]*web.SearchResult, 0, len(s.Data))
	for _, searchResult := range s.Data {
		results = append(results, &web.SearchResult{
			Title:         searchResult.Title,
			URL:           searchResult.URL,
			Snippet:       searchResult.snippet(),
			PublishedTime: parseDate(searchResult.Date),
		})
	}
	return &web.SearchResponse{Query: query, Results: results}
}

func (s *searchResult) snippet() string {
	if s.Description != "" {
		return s.Description
	}
	if s.Content == "" {
		return ""
	}
	if len(s.Content) > 300 {
		return s.Content[:300] + "..."
	}
	return s.Content
}

func parseDate(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{"Jan 2, 2006", "02 Jan 2006", time.DateOnly, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
