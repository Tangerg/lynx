package brave

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/Tangerg/lynx/tools/websearch"
	"github.com/Tangerg/lynx/tools/websearch/internal/queryparam"
)

const (
	Name    = "brave"
	baseURL = "https://api.search.brave.com/res/v1"
)

type Config struct {
	APIKey string
}

func (c *Config) Validate() error {
	if c == nil {
		return errors.New("brave: Config must not be nil")
	}
	if c.APIKey == "" {
		return errors.New("brave: APIKey is required")
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
			SetHeader("X-Subscription-Token", cfg.APIKey).
			SetHeader("Accept", "application/json"),
	}, nil
}

func (c *Client) Name() string { return Name }

// ============================================================== Native API

// Request is the full Brave Web Search query-parameter shape. All
// fields ride as URL query parameters on GET /web/search.
type Request struct {
	// Q is the search query. Required, max 400 chars / 50 words.
	Q string `json:"-"`

	// Count caps results per page (max 20, default 20).
	Count int `json:"-"`

	// Offset is the zero-based page offset (max 9).
	Offset int `json:"-"`

	// Country is a 2-character country code (e.g. "US", "GB").
	Country string `json:"-"`

	// SearchLang is an ISO 639-1 language code (e.g. "en").
	SearchLang string `json:"-"`

	// UILang is the UI language preference (e.g. "en-US").
	UILang string `json:"-"`

	// Safesearch: "off", "moderate" (default), or "strict".
	Safesearch string `json:"-"`

	// Freshness limits by recency. Predefined: "pd" (24h), "pw" (7d),
	// "pm" (31d), "py" (year). Custom: "YYYY-MM-DDtoYYYY-MM-DD".
	Freshness string `json:"-"`

	// TextDecorations toggles highlight markup in snippets.
	TextDecorations bool `json:"-"`

	// Spellcheck enables Brave's spelling correction.
	Spellcheck bool `json:"-"`

	// ResultFilter is a comma-separated list of result types to
	// include (e.g. "web,news,videos"). "" = all types.
	ResultFilter string `json:"-"`

	// GogglesID applies a custom re-ranking ruleset.
	GogglesID string `json:"-"`

	// Units: "metric" or "imperial".
	Units string `json:"-"`

	// ExtraSnippets requests up to 5 additional excerpts per result.
	ExtraSnippets bool `json:"-"`

	// Summary enables Brave's AI summarizer (requires Pro plan).
	Summary bool `json:"-"`
}

func (r *Request) Validate() error {
	if r == nil {
		return errors.New("brave: Request must not be nil")
	}
	if r.Q == "" {
		return errors.New("brave: Q must not be empty")
	}
	return nil
}

// Result is one item in [WebResults.Results].
type Result struct {
	Title          string   `json:"title"`
	URL            string   `json:"url"`
	Description    string   `json:"description"`
	Age            string   `json:"age,omitempty"`
	PageAge        string   `json:"page_age,omitempty"`
	ExtraSnippets  []string `json:"extra_snippets,omitempty"`
	Language       string   `json:"language,omitempty"`
	FamilyFriendly bool     `json:"family_friendly,omitempty"`
}

// WebResults wraps the web vertical of the response.
type WebResults struct {
	Type    string    `json:"type"`
	Results []*Result `json:"results"`
}

// QueryInfo echoes the executed query and pagination hints.
type QueryInfo struct {
	Original             string `json:"original"`
	MoreResultsAvailable bool   `json:"more_results_available,omitempty"`
	Country              string `json:"country,omitempty"`
	IsNavigational       bool   `json:"is_navigational,omitempty"`
	SpellcheckOff        bool   `json:"spellcheck_off,omitempty"`
}

// Response is the full Brave Web Search response. Only the web
// vertical is surfaced today; news/videos/places use their own
// sub-objects on the wire — model them with `any` if you need them.
type Response struct {
	Type  string      `json:"type"`
	Query QueryInfo   `json:"query"`
	Web   *WebResults `json:"web,omitempty"`
	Mixed any         `json:"mixed,omitempty"`
}

// SearchNative calls GET /web/search with the full Brave request.
func (c *Client) SearchNative(ctx context.Context, req *Request) (*Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	var raw Response
	resp, err := c.http.R().SetContext(ctx).SetQueryParams(toQueryParams(req)).SetResult(&raw).Get("/web/search")
	if err != nil {
		return nil, fmt.Errorf("brave: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("brave: API error (status %d): %s", resp.StatusCode(), resp.String())
	}
	return &raw, nil
}

func toQueryParams(r *Request) map[string]string {
	p := map[string]string{"q": r.Q}
	queryparam.AddInt(p, "count", r.Count)
	queryparam.AddInt(p, "offset", r.Offset)
	queryparam.AddStr(p, "country", r.Country)
	queryparam.AddStr(p, "search_lang", r.SearchLang)
	queryparam.AddStr(p, "ui_lang", r.UILang)
	queryparam.AddStr(p, "safesearch", r.Safesearch)
	queryparam.AddStr(p, "freshness", r.Freshness)
	queryparam.AddBool(p, "text_decorations", r.TextDecorations)
	queryparam.AddBool(p, "spellcheck", r.Spellcheck)
	queryparam.AddStr(p, "result_filter", r.ResultFilter)
	queryparam.AddStr(p, "goggles_id", r.GogglesID)
	queryparam.AddStr(p, "units", r.Units)
	queryparam.AddBool(p, "extra_snippets", r.ExtraSnippets)
	queryparam.AddBool(p, "summary", r.Summary)
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

// maxResultsCap matches Brave's documented per-page upper bound.
const maxResultsCap = 20

func buildRequest(req *websearch.Request) *Request {
	r := &Request{
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

func shapeResponse(query string, raw *Response) *websearch.Response {
	var results []*websearch.Result
	if raw.Web != nil {
		results = make([]*websearch.Result, 0, len(raw.Web.Results))
		for _, r := range raw.Web.Results {
			results = append(results, &websearch.Result{
				Title:         r.Title,
				URL:           r.URL,
				Snippet:       r.Description,
				PublishedTime: parseAge(r.PageAge),
			})
		}
	}
	return &websearch.Response{Query: cmp.Or(raw.Query.Original, query), Results: results}
}

// parseAge tries Brave's page_age (RFC3339) format. Relative strings
// like "2 hours ago" — common in Brave's `age` field — are not
// parsed; callers needing them should read [Result.Age] directly.
func parseAge(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return time.Time{}
}
