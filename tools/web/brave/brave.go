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

	"github.com/Tangerg/lynx/tools/web"
)

const (
	baseURL = "https://api.search.brave.com/res/v1"
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
func NewClient(cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("brave: APIKey is required")
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
			SetHeader("X-Subscription-Token", cfg.APIKey).
			SetHeader("Accept", "application/json"),
	}, nil
}

type searchRequest struct {
	Q         string `json:"-"`
	Count     int    `json:"-"`
	Freshness string `json:"-"`
}

func (r *searchRequest) validate() error {
	if r == nil {
		return errors.New("brave: Request must not be nil")
	}
	if r.Q == "" {
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

func (r *searchRequest) params() map[string]string {
	p := map[string]string{"q": r.Q}
	addIntParam(p, "count", r.Count)
	addStringParam(p, "freshness", r.Freshness)
	return p
}

func addStringParam(params map[string]string, key, value string) {
	if value != "" {
		params[key] = value
	}
}

func addIntParam(params map[string]string, key string, value int) {
	if value > 0 {
		params[key] = strconv.Itoa(value)
	}
}

func (c *Client) Search(ctx context.Context, req *web.SearchRequest) (*web.SearchResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("brave: %w", err)
	}
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

func (r *searchResponse) toSearchResponse(query string) *web.SearchResponse {
	var results []*web.SearchResult
	if r.Web != nil {
		results = make([]*web.SearchResult, 0, len(r.Web.Results))
		for _, searchResult := range r.Web.Results {
			results = append(results, &web.SearchResult{
				Title:         searchResult.Title,
				URL:           searchResult.URL,
				Snippet:       searchResult.Description,
				PublishedTime: parseAge(searchResult.PageAge),
			})
		}
	}
	return &web.SearchResponse{Query: cmp.Or(r.Query.Original, query), Results: results}
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
