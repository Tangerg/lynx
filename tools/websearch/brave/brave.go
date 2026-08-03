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

	"github.com/Tangerg/lynx/tools/websearch"
)

const (
	Name    = "brave"
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

var _ websearch.Provider = (*Client)(nil)

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

func (c *Client) Name() string { return Name }

type request struct {
	Q         string `json:"-"`
	Count     int    `json:"-"`
	Freshness string `json:"-"`
}

func (r *request) validate() error {
	if r == nil {
		return errors.New("brave: Request must not be nil")
	}
	if r.Q == "" {
		return errors.New("brave: Q must not be empty")
	}
	return nil
}

type result struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
	PageAge     string `json:"page_age,omitempty"`
}

type webResults struct {
	Results []*result `json:"results"`
}

type queryInfo struct {
	Original string `json:"original"`
}

type response struct {
	Query queryInfo   `json:"query"`
	Web   *webResults `json:"web,omitempty"`
}

func (c *Client) search(ctx context.Context, req *request) (*response, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}
	var raw response
	resp, err := c.http.R().SetContext(ctx).SetQueryParams(req.params()).SetResult(&raw).Get("/web/search")
	if err != nil {
		return nil, fmt.Errorf("brave: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("brave: API error (status %d): %s", resp.StatusCode(), resp.String())
	}
	return &raw, nil
}

func (r *request) params() map[string]string {
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

func (c *Client) Search(ctx context.Context, req *websearch.Request) (*websearch.Response, error) {
	raw, err := c.search(ctx, buildRequest(req))
	if err != nil {
		return nil, err
	}
	return raw.toWebSearch(req.Query), nil
}

// maxResultsCap matches Brave's documented per-page upper bound.
const maxResultsCap = 20

func buildRequest(req *websearch.Request) *request {
	r := &request{
		Q:     websearch.BuildSiteOperatorQuery(req.Query, req.AllowedDomains, req.BlockedDomains),
		Count: min(cmp.Or(req.MaxResults, 10), maxResultsCap),
	}
	r.Freshness = recencyToFreshness(req.Recency)
	return r
}

func recencyToFreshness(r websearch.Recency) string {
	switch r {
	case websearch.RecencyHour, websearch.RecencyDay:
		return "pd"
	case websearch.RecencyWeek:
		return "pw"
	case websearch.RecencyMonth:
		return "pm"
	case websearch.RecencyYear:
		return "py"
	}
	return ""
}

func (r *response) toWebSearch(query string) *websearch.Response {
	var results []*websearch.Result
	if r.Web != nil {
		results = make([]*websearch.Result, 0, len(r.Web.Results))
		for _, result := range r.Web.Results {
			results = append(results, &websearch.Result{
				Title:         result.Title,
				URL:           result.URL,
				Snippet:       result.Description,
				PublishedTime: parseAge(result.PageAge),
			})
		}
	}
	return &websearch.Response{Query: cmp.Or(r.Query.Original, query), Results: results}
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
