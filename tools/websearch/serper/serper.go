package serper

import (
	"context"
	"errors"
	"fmt"
	"net/http"
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
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
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
	if cfg.BaseURL == "" {
		cfg.BaseURL = baseURL
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{}
	}
	return &Client{
		http: resty.NewWithClient(cfg.HTTPClient).
			SetBaseURL(cfg.BaseURL).
			SetHeader("X-API-KEY", cfg.APIKey).
			SetHeader("Content-Type", "application/json"),
	}, nil
}

func (c *Client) Name() string { return Name }

type request struct {
	Q           string `json:"q"`
	Num         int    `json:"num,omitempty"`
	Autocorrect bool   `json:"autocorrect,omitempty"`
	Tbs         string `json:"tbs,omitempty"`
}

func (r *request) validate() error {
	if r == nil {
		return errors.New("serper: Request must not be nil")
	}
	if r.Q == "" {
		return errors.New("serper: Q must not be empty")
	}
	return nil
}

type searchParameters struct {
	Q string `json:"q"`
}

type organicResult struct {
	Title   string `json:"title"`
	Link    string `json:"link"`
	Snippet string `json:"snippet"`
	Date    string `json:"date,omitempty"`
}

type response struct {
	SearchParameters searchParameters `json:"searchParameters"`
	Organic          []*organicResult `json:"organic"`
}

func (c *Client) search(ctx context.Context, req *request) (*response, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}
	var raw response
	resp, err := c.http.R().SetContext(ctx).SetBody(req).SetResult(&raw).Post("/search")
	if err != nil {
		return nil, fmt.Errorf("serper: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("serper: API error (status %d): %s", resp.StatusCode(), resp.String())
	}
	return &raw, nil
}

func (c *Client) Search(ctx context.Context, req *websearch.Request) (*websearch.Response, error) {
	raw, err := c.search(ctx, buildRequest(req))
	if err != nil {
		return nil, err
	}
	return raw.toWebSearch(), nil
}

func buildRequest(req *websearch.Request) *request {
	r := &request{
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

func (r *response) toWebSearch() *websearch.Response {
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
