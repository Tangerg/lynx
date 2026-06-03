package jina

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/Tangerg/lynx/tools/websearch"
	"github.com/Tangerg/lynx/tools/websearch/internal/queryparam"
)

const (
	Name    = "jina"
	baseURL = "https://s.jina.ai"
)

type Config struct {
	APIKey string
}

func (c *Config) Validate() error {
	if c == nil {
		return errors.New("jina: Config must not be nil")
	}
	if c.APIKey == "" {
		return errors.New("jina: APIKey is required")
	}
	return nil
}

type Client struct {
	http *resty.Client
}

var _ websearch.Provider = (*Client)(nil)

func NewClient(cfg *Config) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Client{
		http: resty.New().
			SetAuthToken(cfg.APIKey).
			SetHeader("Accept", "application/json").
			SetHeader("X-Respond-With", "no-content"),
	}, nil
}

func (c *Client) Name() string { return Name }

// ============================================================== Native API

// Request is the full Jina /search query-parameter shape. Jina takes
// the query in the URL path; this struct holds everything else.
type Request struct {
	// Query becomes the URL path segment (url-encoded). Required.
	Query string `json:"-"`

	// Type: "web" (default), "images", "news".
	Type string `json:"type,omitempty"`

	// Num / Count cap results in [1, 20] (Jina accepts either name).
	Num   int `json:"num,omitempty"`
	Count int `json:"count,omitempty"`

	// Page is 1-based pagination.
	Page int `json:"page,omitempty"`

	// Provider / Engine select the upstream SERP backend (google, bing,
	// reader). They're kept in sync for compatibility.
	Provider string `json:"provider,omitempty"`
	Engine   string `json:"engine,omitempty"`

	// Geographic / language hints.
	Gl       string `json:"gl,omitempty"`
	Hl       string `json:"hl,omitempty"`
	Location string `json:"location,omitempty"`

	// Fallback lets Jina retry on alternative engines.
	Fallback bool `json:"fallback,omitempty"`

	// Nfpr enables Google's "no filter for personalised results".
	Nfpr bool `json:"nfpr,omitempty"`

	// Search operators — exact behavior matches Google's site:/intitle:.
	Ext      []string `json:"ext,omitempty"`
	Filetype []string `json:"filetype,omitempty"`
	Intitle  []string `json:"intitle,omitempty"`
	Loc      []string `json:"loc,omitempty"`
	Site     []string `json:"site,omitempty"`

	// RespondWith: markdown (default), html, text, screenshot.
	RespondWith string `json:"respondWith,omitempty"`
	// RetainImages / RetainLinks: none, all, alt (images), text (links).
	RetainImages string `json:"retainImages,omitempty"`
	RetainLinks  string `json:"retainLinks,omitempty"`
	// NoCache forces a fresh crawl.
	NoCache bool `json:"noCache,omitempty"`
	// Timeout caps the upstream fetch at this many seconds (max 180).
	Timeout int `json:"timeout,omitempty"`
}

func (r *Request) Validate() error {
	if r == nil {
		return errors.New("jina: Request must not be nil")
	}
	if r.Query == "" {
		return errors.New("jina: Query must not be empty")
	}
	return nil
}

// Usage echoes token consumption per request.
type Usage struct {
	Tokens int `json:"tokens"`
}

// Result is one item in [Response.Data].
type Result struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Content     string `json:"content,omitempty"`
	Date        string `json:"date,omitempty"`
	Usage       Usage  `json:"usage,omitempty"`
}

// ResponseMeta wraps per-call metadata.
type ResponseMeta struct {
	Usage Usage `json:"usage"`
}

// Response is the full Jina /search response.
type Response struct {
	Code    int          `json:"code"`
	Status  int          `json:"status"`
	Data    []*Result    `json:"data"`
	Meta    ResponseMeta `json:"meta"`
	Message string       `json:"message,omitempty"`
}

// SearchNative calls GET https://s.jina.ai/<query> with the full
// Jina query-parameter shape. Note: Query is encoded into the URL
// path, not the query string.
func (c *Client) SearchNative(ctx context.Context, req *Request) (*Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	endpoint := baseURL + "/" + url.PathEscape(req.Query)
	params := toQueryParams(req)

	var raw Response
	resp, err := c.http.R().SetContext(ctx).SetQueryParams(params).SetResult(&raw).Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("jina: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("jina: API error (status %d): %s", resp.StatusCode(), resp.String())
	}
	return &raw, nil
}

// toQueryParams serializes a [Request] into the flat string map resty
// expects. Empty / zero fields are omitted.
func toQueryParams(r *Request) map[string]string {
	p := map[string]string{}
	queryparam.AddStr(p, "type", r.Type)
	queryparam.AddInt(p, "num", r.Num)
	queryparam.AddInt(p, "count", r.Count)
	queryparam.AddInt(p, "page", r.Page)
	queryparam.AddStr(p, "provider", r.Provider)
	queryparam.AddStr(p, "engine", r.Engine)
	queryparam.AddStr(p, "gl", r.Gl)
	queryparam.AddStr(p, "hl", r.Hl)
	queryparam.AddStr(p, "location", r.Location)
	queryparam.AddBool(p, "fallback", r.Fallback)
	queryparam.AddBool(p, "nfpr", r.Nfpr)
	queryparam.AddCSV(p, "ext", r.Ext)
	queryparam.AddCSV(p, "filetype", r.Filetype)
	queryparam.AddCSV(p, "intitle", r.Intitle)
	queryparam.AddCSV(p, "loc", r.Loc)
	queryparam.AddCSV(p, "site", r.Site)
	queryparam.AddStr(p, "respondWith", r.RespondWith)
	queryparam.AddStr(p, "retainImages", r.RetainImages)
	queryparam.AddStr(p, "retainLinks", r.RetainLinks)
	queryparam.AddBool(p, "noCache", r.NoCache)
	// Jina caps timeout at 180s; clamp here so callers can't smuggle
	// values past the limit via the native API.
	if r.Timeout > 0 && r.Timeout <= 180 {
		p["timeout"] = strconv.Itoa(r.Timeout)
	}
	return p
}

// ============================================================== SPI wrapper

func (c *Client) Search(ctx context.Context, req *websearch.Request) (*websearch.Response, error) {
	raw, err := c.SearchNative(ctx, buildRequest(req))
	if err != nil {
		return nil, err
	}
	return shapeResponse(req.Query, raw), nil
}

// ============================================================== mapping

const maxResultsCap = 20

func buildRequest(req *websearch.Request) *Request {
	num := min(cmp.Or(req.MaxResults, 10), maxResultsCap)
	r := &Request{
		Query:        req.Query,
		Type:         "web",
		Provider:     "google",
		Engine:       "google",
		Fallback:     true,
		Num:          num,
		Count:        num,
		Page:         1,
		RespondWith:  "markdown",
		RetainImages: "none",
		RetainLinks:  "all",
	}
	if len(req.AllowedDomains) > 0 {
		r.Site = req.AllowedDomains
	}
	if req.Recency != "" {
		r.NoCache = true
	}
	return r
}

func shapeResponse(query string, raw *Response) *websearch.Response {
	results := make([]*websearch.Result, 0, len(raw.Data))
	for _, r := range raw.Data {
		results = append(results, &websearch.Result{
			Title:         r.Title,
			URL:           r.URL,
			Snippet:       pickSnippet(r),
			PublishedTime: parseDate(r.Date),
		})
	}
	return &websearch.Response{Query: query, Results: results}
}

func pickSnippet(r *Result) string {
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
