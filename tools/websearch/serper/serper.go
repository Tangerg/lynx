package serper

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/Tangerg/lynx/tools/websearch"
)

const (
	Name    = "serper"
	baseURL = "https://google.serper.dev"
)

// Config configures [NewClient].
type Config struct {
	APIKey string
}

type Client struct {
	http *resty.Client
}

var _ websearch.Provider = (*Client)(nil)

// NewClient returns a Serper-backed client.
func NewClient(cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("serper: APIKey is required")
	}
	return &Client{
		http: resty.New().
			SetBaseURL(baseURL).
			SetHeader("X-API-KEY", cfg.APIKey).
			SetHeader("Content-Type", "application/json"),
	}, nil
}

func (c *Client) Name() string { return Name }

// ============================================================== Native API

// Request is the full Serper /search request shape.
type Request struct {
	// Q is the Google query (supports site:/-site:, intitle:, etc.).
	Q string `json:"q"`

	// Gl is the ISO 3166-1 alpha-2 country code (lowercase, e.g. "us").
	Gl string `json:"gl,omitempty"`

	// Hl is the ISO 639-1 language code (lowercase, e.g. "en").
	Hl string `json:"hl,omitempty"`

	// Num is results per page; default 10.
	Num int `json:"num,omitempty"`

	// Page is 1-based pagination.
	Page int `json:"page,omitempty"`

	// Autocorrect toggles Google's spelling correction.
	Autocorrect bool `json:"autocorrect,omitempty"`

	// Location is a fine-grained geo target (e.g. "London, UK").
	Location string `json:"location,omitempty"`

	// Tbs is Google's time-based search filter, e.g. "qdr:d" or
	// "cdr:1,cd_min:01/01/2026,cd_max:01/31/2026".
	Tbs string `json:"tbs,omitempty"`
}

func (r *Request) Validate() error {
	if r == nil {
		return errors.New("serper: Request must not be nil")
	}
	if r.Q == "" {
		return errors.New("serper: Q must not be empty")
	}
	return nil
}

// SearchParameters echoes the parameters Serper actually used.
type SearchParameters struct {
	Q      string `json:"q"`
	Type   string `json:"type,omitempty"`
	Engine string `json:"engine,omitempty"`
	Gl     string `json:"gl,omitempty"`
	Hl     string `json:"hl,omitempty"`
}

// Sitelink is a nested link block under an organic result.
type Sitelink struct {
	Title string `json:"title"`
	Link  string `json:"link"`
}

// Organic is one item in [Response.Organic].
type Organic struct {
	Title     string      `json:"title"`
	Link      string      `json:"link"`
	Snippet   string      `json:"snippet"`
	Position  int         `json:"position"`
	Date      string      `json:"date,omitempty"`
	Sitelinks []*Sitelink `json:"sitelinks,omitempty"`
}

// AnswerBox is Google's "direct answer" block when present.
type AnswerBox struct {
	Title   string `json:"title,omitempty"`
	Link    string `json:"link,omitempty"`
	Snippet string `json:"snippet,omitempty"`
	Answer  string `json:"answer,omitempty"`
}

// KnowledgeGraph is Google's entity panel when present.
type KnowledgeGraph struct {
	Title             string            `json:"title,omitempty"`
	Type              string            `json:"type,omitempty"`
	Website           string            `json:"website,omitempty"`
	Description       string            `json:"description,omitempty"`
	DescriptionSource string            `json:"descriptionSource,omitempty"`
	DescriptionLink   string            `json:"descriptionLink,omitempty"`
	Attributes        map[string]string `json:"attributes,omitempty"`
	ImageURL          string            `json:"imageUrl,omitempty"`
}

// PeopleAlsoAsk is one entry in the "People also ask" panel.
type PeopleAlsoAsk struct {
	Question string `json:"question"`
	Snippet  string `json:"snippet,omitempty"`
	Title    string `json:"title,omitempty"`
	Link     string `json:"link,omitempty"`
}

// RelatedSearch is one entry under "Related searches".
type RelatedSearch struct {
	Query string `json:"query"`
}

// Response is the full Serper /search response.
type Response struct {
	SearchParameters SearchParameters `json:"searchParameters"`
	AnswerBox        *AnswerBox       `json:"answerBox,omitempty"`
	KnowledgeGraph   *KnowledgeGraph  `json:"knowledgeGraph,omitempty"`
	Organic          []*Organic       `json:"organic"`
	PeopleAlsoAsk    []*PeopleAlsoAsk `json:"peopleAlsoAsk,omitempty"`
	RelatedSearches  []*RelatedSearch `json:"relatedSearches,omitempty"`
	Credits          int              `json:"credits"`
}

// SearchNative calls POST /search with the full Serper request shape.
func (c *Client) SearchNative(ctx context.Context, req *Request) (*Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	var raw Response
	resp, err := c.http.R().SetContext(ctx).SetBody(req).SetResult(&raw).Post("/search")
	if err != nil {
		return nil, fmt.Errorf("serper: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("serper: API error (status %d): %s", resp.StatusCode(), resp.String())
	}
	return &raw, nil
}

// ============================================================== SPI wrapper

func (c *Client) Search(ctx context.Context, req *websearch.Request) (*websearch.Response, error) {
	raw, err := c.SearchNative(ctx, buildRequest(req))
	if err != nil {
		return nil, err
	}
	return raw.toWebSearch(), nil
}

// ============================================================== mapping

func buildRequest(req *websearch.Request) *Request {
	r := &Request{
		Q:           websearch.BuildSiteOperatorQuery(req.Query, req.AllowedDomains, req.BlockedDomains),
		Autocorrect: true,
	}
	if req.MaxResults > 0 {
		r.Num = req.MaxResults
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

func (r *Response) toWebSearch() *websearch.Response {
	results := make([]*websearch.Result, 0, len(r.Organic))
	for _, result := range r.Organic {
		results = append(results, &websearch.Result{
			Title:         result.Title,
			URL:           result.Link,
			Snippet:       result.Snippet,
			PublishedTime: parseDate(result.Date),
		})
	}
	return &websearch.Response{Query: r.SearchParameters.Q, Results: results}
}

// parseDate tries Serper's common date formats. Relative strings
// ("2 days ago") are returned as zero time.
func parseDate(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{"Jan 2, 2006", time.DateOnly, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
