package firecrawl

import (
	"cmp"
	"context"
	"errors"
	"fmt"

	"github.com/go-resty/resty/v2"

	"github.com/Tangerg/lynx/tools/websearch"
)

const (
	Name    = "firecrawl"
	baseURL = "https://api.firecrawl.dev/v2"
)

type Config struct {
	APIKey string
}

func (c *Config) Validate() error {
	if c == nil {
		return errors.New("firecrawl: Config must not be nil")
	}
	if c.APIKey == "" {
		return errors.New("firecrawl: APIKey is required")
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
			SetBaseURL(baseURL).
			SetAuthToken(cfg.APIKey).
			SetHeader("Content-Type", "application/json"),
	}, nil
}

func (c *Client) Name() string { return Name }

// ============================================================== Native API

// Source picks which Google vertical(s) Firecrawl scrapes.
type Source struct {
	Type string `json:"type"` // "web", "news", "images"
}

// ScrapeFormat is one entry in [ScrapeOptions.Formats].
type ScrapeFormat struct {
	Type string `json:"type"` // "markdown", "html", "rawHtml", "links", "screenshot"
}

// ScrapeOptions, when set, asks Firecrawl to scrape each search hit
// and populate the matching fields on every result.
type ScrapeOptions struct {
	Formats         []ScrapeFormat `json:"formats,omitempty"`
	OnlyMainContent bool           `json:"onlyMainContent,omitempty"`
	IncludeTags     []string       `json:"includeTags,omitempty"`
	ExcludeTags     []string       `json:"excludeTags,omitempty"`
	WaitFor         int            `json:"waitFor,omitempty"`
	Mobile          bool           `json:"mobile,omitempty"`
	Timeout         int            `json:"timeout,omitempty"`
}

// Request is the full Firecrawl /v2/search request shape.
type Request struct {
	// Query is the search query. Required, max 500 chars.
	Query string `json:"query"`

	// Limit caps results per source in [1, 100]; default 10.
	Limit int `json:"limit,omitempty"`

	// Country is an ISO 3166-1 alpha-2 code (default "US").
	Country string `json:"country,omitempty"`

	// Location is a fine-grained geo target
	// (e.g. "San Francisco,California,United States").
	Location string `json:"location,omitempty"`

	// Tbs is Google's tbs parameter — "qdr:h"/"qdr:d"/"qdr:w" etc.
	Tbs string `json:"tbs,omitempty"`

	// Sources picks one or more verticals. Default [{"type":"web"}].
	Sources []Source `json:"sources,omitempty"`

	// ScrapeOptions, when present, runs a /scrape on each result and
	// inlines the rendered content into the response.
	ScrapeOptions *ScrapeOptions `json:"scrapeOptions,omitempty"`
}

func (r *Request) Validate() error {
	if r == nil {
		return errors.New("firecrawl: Request must not be nil")
	}
	if r.Query == "" {
		return errors.New("firecrawl: Query must not be empty")
	}
	return nil
}

// ResultMetadata is the metadata block on each hit when scraping.
type ResultMetadata struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	SourceURL   string `json:"sourceURL,omitempty"`
	URL         string `json:"url,omitempty"`
	StatusCode  int    `json:"statusCode,omitempty"`
	Error       string `json:"error,omitempty"`
}

// Result is one item in [ResponseData.Web] / .News / .Images.
type Result struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
	// The fields below are populated only when ScrapeOptions is set.
	Markdown   *string         `json:"markdown,omitempty"`
	HTML       *string         `json:"html,omitempty"`
	RawHTML    *string         `json:"rawHtml,omitempty"`
	Links      []string        `json:"links,omitempty"`
	Screenshot *string         `json:"screenshot,omitempty"`
	Metadata   *ResultMetadata `json:"metadata,omitempty"`
}

// ResponseData holds per-vertical result arrays.
type ResponseData struct {
	Web    []*Result `json:"web,omitempty"`
	News   []*Result `json:"news,omitempty"`
	Images []*Result `json:"images,omitempty"`
}

// Response is the full Firecrawl /v2/search response.
type Response struct {
	Success     bool         `json:"success"`
	Data        ResponseData `json:"data"`
	Warning     string       `json:"warning,omitempty"`
	ID          string       `json:"id,omitempty"`
	CreditsUsed int          `json:"creditsUsed,omitempty"`
}

// SearchNative calls POST /search with the full Firecrawl request shape.
func (c *Client) SearchNative(ctx context.Context, req *Request) (*Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	var raw Response
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

// ============================================================== SPI wrapper

func (c *Client) Search(ctx context.Context, req *websearch.Request) (*websearch.Response, error) {
	raw, err := c.SearchNative(ctx, buildRequest(req))
	if err != nil {
		return nil, err
	}
	return shapeResponse(req.Query, raw), nil
}

// ============================================================== mapping

const maxResultsCap = 100

func buildRequest(req *websearch.Request) *Request {
	r := &Request{
		Query: websearch.BuildSiteOperatorQuery(req.Query, req.AllowedDomains, req.BlockedDomains),
		Limit: min(cmp.Or(req.MaxResults, 10), maxResultsCap),
	}
	r.Tbs = recencyToTbs(req.Recency)
	return r
}

func recencyToTbs(r websearch.Recency) string {
	switch r {
	case websearch.RecencyHour:
		return "qdr:h"
	case websearch.RecencyDay:
		return "qdr:d"
	case websearch.RecencyWeek:
		return "qdr:w"
	case websearch.RecencyMonth:
		return "qdr:m"
	case websearch.RecencyYear:
		return "qdr:y"
	}
	return ""
}

func shapeResponse(query string, raw *Response) *websearch.Response {
	results := make([]*websearch.Result, 0, len(raw.Data.Web))
	for _, r := range raw.Data.Web {
		results = append(results, &websearch.Result{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Description,
		})
	}
	return &websearch.Response{Query: query, Results: results}
}
