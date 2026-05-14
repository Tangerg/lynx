package exa

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-resty/resty/v2"

	"github.com/Tangerg/lynx/tools/webfetch"
)

const (
	Name    = "exa"
	baseURL = "https://api.exa.ai"
)

type Config struct {
	APIKey string
}

func (c *Config) Validate() error {
	if c == nil {
		return errors.New("exa: Config must not be nil")
	}
	if c.APIKey == "" {
		return errors.New("exa: APIKey is required")
	}
	return nil
}

type Client struct {
	http *resty.Client
}

var _ webfetch.Provider = (*Client)(nil)

func NewClient(cfg *Config) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Client{
		http: resty.New().
			SetBaseURL(baseURL).
			SetHeader("x-api-key", cfg.APIKey).
			SetHeader("Content-Type", "application/json"),
	}, nil
}

func (c *Client) Name() string { return Name }

// ============================================================== Native API

// TextOptions configures text extraction. Set via Request.Text when
// you need more than a plain bool.
type TextOptions struct {
	MaxCharacters   int      `json:"maxCharacters,omitempty"`
	IncludeHTMLTags bool     `json:"includeHtmlTags,omitempty"`
	Verbosity       string   `json:"verbosity,omitempty"` // compact, standard, full
	IncludeSections []string `json:"includeSections,omitempty"`
	ExcludeSections []string `json:"excludeSections,omitempty"`
}

// HighlightsOptions configures snippet extraction.
type HighlightsOptions struct {
	MaxCharacters int    `json:"maxCharacters,omitempty"`
	Query         string `json:"query,omitempty"`
}

// SummaryOptions configures LLM-generated summaries.
type SummaryOptions struct {
	Query  string         `json:"query,omitempty"`
	Schema map[string]any `json:"schema,omitempty"`
}

// ExtrasOptions requests additional data.
type ExtrasOptions struct {
	Links      int `json:"links,omitempty"`
	ImageLinks int `json:"imageLinks,omitempty"`
}

// Request is the full Exa /contents request shape.
type Request struct {
	// URLs is the list of URLs to fetch. At least one required (or use IDs).
	URLs []string `json:"urls,omitempty"`

	// IDs is the alternative form using Exa document IDs from a prior
	// search call.
	IDs []string `json:"ids,omitempty"`

	// Text: bool (default settings) or *TextOptions (custom).
	Text any `json:"text,omitempty"`

	// Highlights: bool (defaults) or *HighlightsOptions.
	Highlights any `json:"highlights,omitempty"`

	// Summary controls LLM summary extraction.
	Summary *SummaryOptions `json:"summary,omitempty"`

	// LiveCrawlTimeout in ms (default 10000).
	LiveCrawlTimeout int `json:"livecrawlTimeout,omitempty"`

	// MaxAgeHours: cache freshness gate.
	//   > 0 : use cache if fresher than N hours, else live-crawl
	//   0   : always live-crawl
	//   -1  : never live-crawl
	MaxAgeHours *int `json:"maxAgeHours,omitempty"`

	// Subpages caps subpage crawls per URL.
	Subpages int `json:"subpages,omitempty"`

	// SubpageTarget is a string or []string filter for subpage discovery.
	SubpageTarget any `json:"subpageTarget,omitempty"`

	// Extras controls links / imageLinks extraction.
	Extras *ExtrasOptions `json:"extras,omitempty"`
}

// Validate enforces request invariants. Exa accepts either URLs or
// IDs (or both); at least one must be non-empty.
func (r *Request) Validate() error {
	if r == nil {
		return errors.New("exa: Request must not be nil")
	}
	if len(r.URLs) == 0 && len(r.IDs) == 0 {
		return errors.New("exa: URLs or IDs must be non-empty")
	}
	return nil
}

// Result is one entry in [Response.Results].
type Result struct {
	ID            string    `json:"id"`
	URL           string    `json:"url"`
	Title         string    `json:"title,omitempty"`
	PublishedDate string    `json:"publishedDate,omitempty"`
	Author        string    `json:"author,omitempty"`
	Image         string    `json:"image,omitempty"`
	Favicon       string    `json:"favicon,omitempty"`
	Text          string    `json:"text,omitempty"`
	Highlights    []string  `json:"highlights,omitempty"`
	Summary       string    `json:"summary,omitempty"`
	Subpages      []*Result `json:"subpages,omitempty"`
}

// StatusError holds error details for a failed URL fetch.
type StatusError struct {
	Tag            string `json:"tag"`
	HTTPStatusCode *int   `json:"httpStatusCode,omitempty"`
}

// URLStatus holds the per-URL fetch status.
type URLStatus struct {
	ID     string       `json:"id"`
	Status string       `json:"status"`
	Error  *StatusError `json:"error,omitempty"`
}

// Response is the full Exa /contents response.
type Response struct {
	RequestID string       `json:"requestId"`
	Results   []*Result    `json:"results"`
	Statuses  []*URLStatus `json:"statuses,omitempty"`
}

// FetchNative calls POST /contents with the full Exa request shape.
func (c *Client) FetchNative(ctx context.Context, req *Request) (*Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	var raw Response
	resp, err := c.http.R().SetContext(ctx).SetBody(req).SetResult(&raw).Post("/contents")
	if err != nil {
		return nil, fmt.Errorf("exa: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("exa: API error (status %d): %s", resp.StatusCode(), resp.String())
	}
	return &raw, nil
}

// ============================================================== SPI wrapper

func (c *Client) Fetch(ctx context.Context, req *webfetch.Request) (*webfetch.Response, error) {
	format := req.Format
	if format == "" {
		format = webfetch.FormatMarkdown
	}
	raw, err := c.FetchNative(ctx, &Request{
		URLs: []string{req.URL},
		Text: &TextOptions{IncludeHTMLTags: format == webfetch.FormatHTML},
	})
	if err != nil {
		return nil, err
	}
	if len(raw.Results) == 0 {
		return nil, fmt.Errorf("exa: empty result for %s", req.URL)
	}
	return &webfetch.Response{Content: raw.Results[0].Text, Format: format}, nil
}
