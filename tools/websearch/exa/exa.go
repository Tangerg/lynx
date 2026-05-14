package exa

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/Tangerg/lynx/tools/websearch"
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

var _ websearch.Provider = (*Client)(nil)

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

// TextOptions configures text extraction in [ContentsOptions].
type TextOptions struct {
	MaxCharacters   int      `json:"maxCharacters,omitempty"`
	IncludeHTMLTags bool     `json:"includeHtmlTags,omitempty"`
	Verbosity       string   `json:"verbosity,omitempty"` // compact, standard, full
	IncludeSections []string `json:"includeSections,omitempty"`
	ExcludeSections []string `json:"excludeSections,omitempty"`
}

// HighlightsOptions configures highlight extraction.
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

// ContentsOptions controls per-result content extraction. Text and
// Highlights accept either bool or their options struct — use any so
// the wire format can carry either.
type ContentsOptions struct {
	Text          any             `json:"text,omitempty"`
	Highlights    any             `json:"highlights,omitempty"`
	Summary       *SummaryOptions `json:"summary,omitempty"`
	MaxAgeHours   *int            `json:"maxAgeHours,omitempty"`
	Subpages      int             `json:"subpages,omitempty"`
	SubpageTarget any             `json:"subpageTarget,omitempty"`
	Extras        *ExtrasOptions  `json:"extras,omitempty"`
}

// Request is the full Exa /search request shape.
type Request struct {
	Query              string           `json:"query"`
	AdditionalQueries  []string         `json:"additionalQueries,omitempty"`
	Type               string           `json:"type,omitempty"`     // neural, fast, auto, deep-lite, deep, deep-reasoning, instant
	Category           string           `json:"category,omitempty"` // company, research paper, news, tweet, personal site, financial report, people
	UserLocation       string           `json:"userLocation,omitempty"`
	NumResults         int              `json:"numResults,omitempty"`
	IncludeDomains     []string         `json:"includeDomains,omitempty"`
	ExcludeDomains     []string         `json:"excludeDomains,omitempty"`
	StartCrawlDate     string           `json:"startCrawlDate,omitempty"`
	EndCrawlDate       string           `json:"endCrawlDate,omitempty"`
	StartPublishedDate string           `json:"startPublishedDate,omitempty"`
	EndPublishedDate   string           `json:"endPublishedDate,omitempty"`
	IncludeText        []string         `json:"includeText,omitempty"`
	ExcludeText        []string         `json:"excludeText,omitempty"`
	Moderation         bool             `json:"moderation,omitempty"`
	Contents           *ContentsOptions `json:"contents,omitempty"`
}

func (r *Request) Validate() error {
	if r == nil {
		return errors.New("exa: Request must not be nil")
	}
	if r.Query == "" {
		return errors.New("exa: Query must not be empty")
	}
	return nil
}

// Result is one item in [Response.Results].
type Result struct {
	Title           string    `json:"title"`
	URL             string    `json:"url"`
	PublishedDate   string    `json:"publishedDate,omitempty"`
	Author          string    `json:"author,omitempty"`
	ID              string    `json:"id,omitempty"`
	Image           string    `json:"image,omitempty"`
	Favicon         string    `json:"favicon,omitempty"`
	Text            string    `json:"text,omitempty"`
	Highlights      []string  `json:"highlights,omitempty"`
	HighlightScores []float64 `json:"highlightScores,omitempty"`
	Summary         string    `json:"summary,omitempty"`
	Subpages        []*Result `json:"subpages,omitempty"`
	Extras          *struct {
		Links      []string `json:"links,omitempty"`
		ImageLinks []string `json:"imageLinks,omitempty"`
	} `json:"extras,omitempty"`
}

// CostBreakdown holds per-operation cost figures.
type CostBreakdown struct {
	NeuralSearch     float64 `json:"neuralSearch,omitempty"`
	DeepSearch       float64 `json:"deepSearch,omitempty"`
	ContentText      float64 `json:"contentText,omitempty"`
	ContentHighlight float64 `json:"contentHighlight,omitempty"`
	ContentSummary   float64 `json:"contentSummary,omitempty"`
}

// CostDollars is the optional monetary cost in the response.
type CostDollars struct {
	Total     float64        `json:"total"`
	BreakDown *CostBreakdown `json:"breakDown,omitempty"`
}

// Response is the full Exa /search response.
type Response struct {
	RequestID   string       `json:"requestId"`
	Results     []*Result    `json:"results"`
	SearchType  string       `json:"searchType,omitempty"`
	CostDollars *CostDollars `json:"costDollars,omitempty"`
}

// SearchNative calls POST /search with the full Exa request shape.
func (c *Client) SearchNative(ctx context.Context, req *Request) (*Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	var raw Response
	resp, err := c.http.R().SetContext(ctx).SetBody(req).SetResult(&raw).Post("/search")
	if err != nil {
		return nil, fmt.Errorf("exa: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("exa: API error (status %d): %s", resp.StatusCode(), resp.String())
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
		Query:      req.Query,
		Type:       "fast",
		NumResults: clampResults(req.MaxResults),
		Contents: &ContentsOptions{
			Summary: &SummaryOptions{Query: req.Query},
		},
	}
	if len(req.AllowedDomains) > 0 {
		r.IncludeDomains = req.AllowedDomains
	}
	if len(req.BlockedDomains) > 0 {
		r.ExcludeDomains = req.BlockedDomains
	}
	if start := recencyToStart(req.Recency); !start.IsZero() {
		r.StartPublishedDate = start.Format(time.RFC3339)
	}
	return r
}

// clampResults applies Exa's [1, 100] bound with a 10 default.
func clampResults(n int) int {
	return min(cmp.Or(n, 10), maxResultsCap)
}

func recencyToStart(r websearch.Recency) time.Time {
	now := time.Now()
	switch r {
	case websearch.RecencyHour:
		return now.Add(-time.Hour)
	case websearch.RecencyDay:
		return now.Add(-24 * time.Hour)
	case websearch.RecencyWeek:
		return now.Add(-7 * 24 * time.Hour)
	case websearch.RecencyMonth:
		return now.AddDate(0, -1, 0)
	case websearch.RecencyYear:
		return now.AddDate(-1, 0, 0)
	}
	return time.Time{}
}

func shapeResponse(query string, raw *Response) *websearch.Response {
	results := make([]*websearch.Result, 0, len(raw.Results))
	for _, r := range raw.Results {
		results = append(results, &websearch.Result{
			Title:         r.Title,
			URL:           r.URL,
			Snippet:       pickSnippet(r),
			FaviconURL:    r.Favicon,
			Source:        r.Author,
			PublishedTime: parseDate(r.PublishedDate),
		})
	}
	return &websearch.Response{Query: query, Results: results}
}

func pickSnippet(r *Result) string {
	if len(r.Highlights) > 0 {
		return r.Highlights[0]
	}
	return r.Summary
}

func parseDate(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return time.Time{}
}
