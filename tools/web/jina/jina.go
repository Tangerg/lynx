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
	"github.com/samber/lo"

	"github.com/Tangerg/scope/tools/web"
)

const (
	searchBaseURL         = "https://s.jina.ai"
	fetchBaseURL          = "https://r.jina.ai"
	queryParameterCount   = "count"
	queryParameterPage    = "page"
	queryParameterSite    = "site"
	queryParameterNoCache = "noCache"
	defaultSearchResults  = 10
	maximumSnippetRunes   = 300
	snippetEllipsis       = "..."
	mediaTypeJSON         = "application/json"
	respondWithHeader     = "X-Respond-With"
	respondWithoutContent = "no-content"
)

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

func NewClient(config Config) (*Client, error) {
	if config.APIKey == "" {
		return nil, errors.New("jina: API key is required")
	}
	if config.SearchBaseURL == "" {
		config.SearchBaseURL = searchBaseURL
	}
	if config.FetchBaseURL == "" {
		config.FetchBaseURL = fetchBaseURL
	}
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{}
	}
	return &Client{
		searchHTTP: resty.NewWithClient(config.HTTPClient).
			SetBaseURL(config.SearchBaseURL).
			SetAuthToken(config.APIKey).
			SetHeader("Accept", mediaTypeJSON).
			SetHeader(respondWithHeader, respondWithoutContent),
		fetchHTTP: resty.NewWithClient(config.HTTPClient).
			SetBaseURL(config.FetchBaseURL).
			SetAuthToken(config.APIKey).
			SetHeader("Content-Type", mediaTypeJSON).
			SetHeader("Accept", mediaTypeJSON),
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
		return errors.New("jina: search request must not be nil")
	}
	if s.Query == "" {
		return errors.New("jina: search query must not be empty")
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

func (c *Client) search(ctx context.Context, request *searchRequest) (*searchResponse, error) {
	if err := request.validate(); err != nil {
		return nil, err
	}
	endpoint := "/" + url.PathEscape(request.Query)
	params := request.params()

	var raw searchResponse
	response, err := c.searchHTTP.R().SetContext(ctx).SetQueryParams(params).SetResult(&raw).Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("jina: execute search request: %w", err)
	}
	if !response.IsSuccess() {
		return nil, fmt.Errorf("jina: search request returned HTTP %d: %s", response.StatusCode(), response.String())
	}
	return &raw, nil
}

func (request *searchRequest) params() map[string]string {
	parameters := make(map[string]string)
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

func (c *Client) Search(ctx context.Context, request *web.SearchRequest) (*web.SearchResponse, error) {
	prepared, err := request.Prepare()
	if err != nil {
		return nil, fmt.Errorf("jina: prepare search request: %w", err)
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
		Query: request.Query,
		Count: cmp.Or(request.MaxResults, defaultSearchResults),
		Page:  1,
	}
	if len(request.AllowedDomains) > 0 {
		r.Site = request.AllowedDomains
	}
	if request.Recency != "" {
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
	if lo.RuneLength(s.Content) > maximumSnippetRunes {
		return lo.Substring(s.Content, 0, maximumSnippetRunes) + snippetEllipsis
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
