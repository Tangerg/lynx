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

	"github.com/Tangerg/lynx/tools/websearch"
)

const (
	Name    = "jina"
	baseURL = "https://s.jina.ai"
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

// NewClient returns a Jina Search-backed client.
func NewClient(cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("jina: APIKey is required")
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
			SetHeader("Accept", "application/json").
			SetHeader("X-Respond-With", "no-content"),
	}, nil
}

func (c *Client) Name() string { return Name }

type request struct {
	Query   string   `json:"-"`
	Count   int      `json:"count,omitempty"`
	Page    int      `json:"page,omitempty"`
	Site    []string `json:"site,omitempty"`
	NoCache bool     `json:"noCache,omitempty"`
}

func (r *request) validate() error {
	if r == nil {
		return errors.New("jina: Request must not be nil")
	}
	if r.Query == "" {
		return errors.New("jina: Query must not be empty")
	}
	return nil
}

type result struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Content     string `json:"content,omitempty"`
	Date        string `json:"date,omitempty"`
}

type response struct {
	Data []*result `json:"data"`
}

func (c *Client) search(ctx context.Context, req *request) (*response, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}
	endpoint := "/" + url.PathEscape(req.Query)
	params := req.params()

	var raw response
	resp, err := c.http.R().SetContext(ctx).SetQueryParams(params).SetResult(&raw).Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("jina: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("jina: API error (status %d): %s", resp.StatusCode(), resp.String())
	}
	return &raw, nil
}

func (r *request) params() map[string]string {
	p := map[string]string{}
	addIntParam(p, "count", r.Count)
	addIntParam(p, "page", r.Page)
	addCSVParam(p, "site", r.Site)
	addBoolParam(p, "noCache", r.NoCache)
	return p
}

func addIntParam(params map[string]string, key string, value int) {
	if value > 0 {
		params[key] = strconv.Itoa(value)
	}
}

func addBoolParam(params map[string]string, key string, value bool) {
	if value {
		params[key] = "true"
	}
}

func addCSVParam(params map[string]string, key string, values []string) {
	if len(values) > 0 {
		params[key] = strings.Join(values, ",")
	}
}

func (c *Client) Search(ctx context.Context, req *websearch.Request) (*websearch.Response, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("jina: %w", err)
	}
	raw, err := c.search(ctx, buildRequest(req))
	if err != nil {
		return nil, err
	}
	return raw.toWebSearch(req.Query), nil
}

func buildRequest(req *websearch.Request) *request {
	r := &request{
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

func (r *response) toWebSearch(query string) *websearch.Response {
	results := make([]*websearch.Result, 0, len(r.Data))
	for _, result := range r.Data {
		results = append(results, &websearch.Result{
			Title:         result.Title,
			URL:           result.URL,
			Snippet:       result.snippet(),
			PublishedTime: parseDate(result.Date),
		})
	}
	return &websearch.Response{Query: query, Results: results}
}

func (r *result) snippet() string {
	if r.Description != "" {
		return r.Description
	}
	if r.Content == "" {
		return ""
	}
	if len(r.Content) > 300 {
		return r.Content[:300] + "..."
	}
	return r.Content
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
