package brave

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/Tangerg/scope/tools/web"
)

const (
	baseURL                 = "https://api.search.brave.com/res/v1"
	queryParameterQuery     = "q"
	queryParameterCount     = "count"
	queryParameterFreshness = "freshness"
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

// NewClient returns a Brave-backed client.
func NewClient(config Config) (*Client, error) {
	if config.APIKey == "" {
		return nil, errors.New("brave: APIKey is required")
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
			SetHeader("X-Subscription-Token", config.APIKey).
			SetHeader("Accept", "application/json"),
	}, nil
}

type searchRequest struct {
	Q         string `json:"-"`
	Count     int    `json:"-"`
	Freshness string `json:"-"`
}

func (s *searchRequest) validate() error {
	if s == nil {
		return errors.New("brave: Request must not be nil")
	}
	if s.Q == "" {
		return errors.New("brave: Q must not be empty")
	}
	return nil
}

type searchResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
	PageAge     string `json:"page_age,omitempty"`
}

type webResults struct {
	Results []*searchResult `json:"results"`
}

type queryInfo struct {
	Original string `json:"original"`
}

type searchResponse struct {
	Query queryInfo   `json:"query"`
	Web   *webResults `json:"web,omitempty"`
}

func (c *Client) search(ctx context.Context, req *searchRequest) (*searchResponse, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}
	var raw searchResponse
	resp, err := c.http.R().SetContext(ctx).SetQueryParams(req.params()).SetResult(&raw).Get("/web/search")
	if err != nil {
		return nil, fmt.Errorf("brave: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("brave: API error (status %d): %s", resp.StatusCode(), resp.String())
	}
	return &raw, nil
}

func (request *searchRequest) params() map[string]string {
	parameters := map[string]string{queryParameterQuery: request.Q}
	if request.Count > 0 {
		parameters[queryParameterCount] = strconv.Itoa(request.Count)
	}
	if request.Freshness != "" {
		parameters[queryParameterFreshness] = request.Freshness
	}
	return parameters
}

func (c *Client) Search(ctx context.Context, req *web.SearchRequest) (*web.SearchResponse, error) {
	prepared, err := req.Prepare()
	if err != nil {
		return nil, fmt.Errorf("brave: %w", err)
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
		Q:     req.QueryWithSiteOperators(),
		Count: cmp.Or(req.MaxResults, 10),
	}
	r.Freshness = recencyToFreshness(req.Recency)
	return r
}

func recencyToFreshness(r web.Recency) string {
	switch r {
	case web.RecencyHour, web.RecencyDay:
		return "pd"
	case web.RecencyWeek:
		return "pw"
	case web.RecencyMonth:
		return "pm"
	case web.RecencyYear:
		return "py"
	}
	return ""
}

func (s *searchResponse) toSearchResponse(query string) *web.SearchResponse {
	var results []*web.SearchResult
	if s.Web != nil {
		results = make([]*web.SearchResult, 0, len(s.Web.Results))
		for _, searchResult := range s.Web.Results {
			results = append(results, &web.SearchResult{
				Title:         searchResult.Title,
				URL:           searchResult.URL,
				Snippet:       searchResult.Description,
				PublishedTime: parseAge(searchResult.PageAge),
			})
		}
	}
	return &web.SearchResponse{Query: cmp.Or(s.Query.Original, query), Results: results}
}

func parseAge(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return time.Time{}
}
