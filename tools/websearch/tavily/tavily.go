package tavily

import (
	"cmp"
	"context"
	"errors"
	"fmt"

	"github.com/go-resty/resty/v2"

	"github.com/Tangerg/lynx/tools/websearch"
)

const (
	Name    = "tavily"
	baseURL = "https://api.tavily.com"
)

// Config holds the Tavily client config.
type Config struct {
	APIKey string
}

// Validate enforces config invariants. Called by [NewClient].
func (c *Config) Validate() error {
	if c == nil {
		return errors.New("tavily: Config must not be nil")
	}
	if c.APIKey == "" {
		return errors.New("tavily: APIKey is required")
	}
	return nil
}

// Client implements [websearch.Provider] against Tavily.
// Use [Client.SearchNative] for full parameter access; [Client.Search]
// is the slimmer lynx-SPI flavour.
type Client struct {
	http *resty.Client
}

var _ websearch.Provider = (*Client)(nil)

// NewClient builds a Tavily-backed [websearch.Provider].
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

// Request is the full Tavily Search request shape. All fields are
// optional except Query.
type Request struct {
	// Query is the search query. Required.
	Query string `json:"query"`

	// SearchDepth: "basic" (default), "advanced", "fast", "ultra-fast".
	SearchDepth string `json:"search_depth,omitempty"`

	// Topic: "general" (default), "news", "finance".
	Topic string `json:"topic,omitempty"`

	// MaxResults caps results in [0, 20]. Default 5.
	MaxResults int `json:"max_results,omitempty"`

	// TimeRange: "day"/"week"/"month"/"year" or single-letter aliases.
	TimeRange string `json:"time_range,omitempty"`

	// StartDate / EndDate as YYYY-MM-DD.
	StartDate string `json:"start_date,omitempty"`
	EndDate   string `json:"end_date,omitempty"`

	// IncludeDomains caps at 300; ExcludeDomains at 150.
	IncludeDomains []string `json:"include_domains,omitempty"`
	ExcludeDomains []string `json:"exclude_domains,omitempty"`

	// Country (e.g. "United States" or "us") biases by region.
	// Only honoured when Topic == "general".
	Country string `json:"country,omitempty"`

	// IncludeFavicon returns favicon URLs on each result.
	IncludeFavicon bool `json:"include_favicon,omitempty"`

	// ChunksPerSource (1-3) applies only when SearchDepth == "advanced".
	ChunksPerSource int `json:"chunks_per_source,omitempty"`

	// IncludeAnswer: bool, "basic", or "advanced". `any` because the
	// API accepts heterogeneous types.
	IncludeAnswer any `json:"include_answer,omitempty"`

	// IncludeRawContent: bool, "markdown", or "text".
	IncludeRawContent any `json:"include_raw_content,omitempty"`

	IncludeImages            bool `json:"include_images,omitempty"`
	IncludeImageDescriptions bool `json:"include_image_descriptions,omitempty"`
	AutoParameters           bool `json:"auto_parameters,omitempty"`
	ExactMatch               bool `json:"exact_match,omitempty"`
	SafeSearch               bool `json:"safe_search,omitempty"`
	IncludeUsage             bool `json:"include_usage,omitempty"`
}

// Validate enforces request invariants. Called by [Client.SearchNative].
func (r *Request) Validate() error {
	if r == nil {
		return errors.New("tavily: Request must not be nil")
	}
	if r.Query == "" {
		return errors.New("tavily: Query must not be empty")
	}
	return nil
}

// Result is one item in [Response.Results].
type Result struct {
	Title      string  `json:"title"`
	URL        string  `json:"url"`
	Content    string  `json:"content"`
	Score      float64 `json:"score"`
	RawContent string  `json:"raw_content,omitempty"`
	Favicon    string  `json:"favicon,omitempty"`
}

// Image is one item in [Response.Images] when IncludeImages is true.
type Image struct {
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

// Usage is the credit breakdown when IncludeUsage is true.
type Usage struct {
	Credits int `json:"credits"`
}

// AutoParameters echoes the auto-picked topic/depth.
type AutoParameters struct {
	Topic       string `json:"topic,omitempty"`
	SearchDepth string `json:"search_depth,omitempty"`
}

// Response is the full Tavily Search response.
type Response struct {
	Query          string          `json:"query"`
	Answer         string          `json:"answer,omitempty"`
	Images         []*Image        `json:"images,omitempty"`
	Results        []*Result       `json:"results"`
	AutoParameters *AutoParameters `json:"auto_parameters,omitempty"`
	ResponseTime   float64         `json:"response_time"`
	Usage          *Usage          `json:"usage,omitempty"`
	RequestID      string          `json:"request_id,omitempty"`
}

// SearchNative calls POST /search with the full Tavily request shape.
// Returns the raw response so callers can access Answer, Images,
// Usage, etc. that the lynx SPI doesn't surface.
func (c *Client) SearchNative(ctx context.Context, req *Request) (*Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	var raw Response
	resp, err := c.http.R().SetContext(ctx).SetBody(req).SetResult(&raw).Post("/search")
	if err != nil {
		return nil, fmt.Errorf("tavily: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("tavily: API error (status %d): %s", resp.StatusCode(), resp.String())
	}
	return &raw, nil
}

// ============================================================== SPI wrapper

func (c *Client) Search(ctx context.Context, req *websearch.Request) (*websearch.Response, error) {
	raw, err := c.SearchNative(ctx, buildRequest(req))
	if err != nil {
		return nil, err
	}
	return shapeResponse(raw), nil
}

// ============================================================== mapping

const maxResultsCap = 20

func buildRequest(req *websearch.Request) *Request {
	r := &Request{
		Query:          req.Query,
		SearchDepth:    "basic",
		Topic:          "general",
		MaxResults:     clampResults(req.MaxResults),
		IncludeFavicon: true,
	}
	if len(req.AllowedDomains) > 0 {
		r.IncludeDomains = req.AllowedDomains
	}
	if len(req.BlockedDomains) > 0 {
		r.ExcludeDomains = req.BlockedDomains
	}
	r.TimeRange = recencyToTimeRange(req.Recency)
	return r
}

// clampResults applies Tavily's [1, 20] bound with a 5 default.
func clampResults(n int) int {
	return min(cmp.Or(n, 5), maxResultsCap)
}

func recencyToTimeRange(r websearch.Recency) string {
	switch r {
	case websearch.RecencyHour, websearch.RecencyDay:
		return "day" // Tavily's minimum granularity
	case websearch.RecencyWeek:
		return "week"
	case websearch.RecencyMonth:
		return "month"
	case websearch.RecencyYear:
		return "year"
	}
	return ""
}

func shapeResponse(raw *Response) *websearch.Response {
	results := make([]*websearch.Result, 0, len(raw.Results))
	for _, r := range raw.Results {
		results = append(results, &websearch.Result{
			Title:      r.Title,
			URL:        r.URL,
			Snippet:    r.Content,
			FaviconURL: r.Favicon,
		})
	}
	return &websearch.Response{Query: raw.Query, Results: results}
}
