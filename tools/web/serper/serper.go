package serper

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/Tangerg/scope/tools/web"
)

const (
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

var _ web.Searcher = (*Client)(nil)

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

type searchRequest struct {
	Q           string `json:"q"`
	Num         int    `json:"num,omitempty"`
	Autocorrect bool   `json:"autocorrect,omitempty"`
	Tbs         string `json:"tbs,omitempty"`
}

func (s *searchRequest) validate() error {
	if s == nil {
		return errors.New("serper: Request must not be nil")
	}
	if s.Q == "" {
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

type searchResponse struct {
	SearchParameters searchParameters `json:"searchParameters"`
	Organic          []*organicResult `json:"organic"`
}

func (c *Client) search(ctx context.Context, req *searchRequest) (*searchResponse, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}
	var raw searchResponse
	resp, err := c.http.R().SetContext(ctx).SetBody(req).SetResult(&raw).Post("/search")
	if err != nil {
		return nil, fmt.Errorf("serper: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("serper: API error (status %d): %s", resp.StatusCode(), resp.String())
	}
	return &raw, nil
}

func (c *Client) Search(ctx context.Context, req *web.SearchRequest) (*web.SearchResponse, error) {
	prepared, err := req.Prepare()
	if err != nil {
		return nil, fmt.Errorf("serper: %w", err)
	}
	req = prepared
	raw, err := c.search(ctx, buildSearchRequest(req))
	if err != nil {
		return nil, err
	}
	return raw.toSearchResponse(), nil
}

func buildSearchRequest(req *web.SearchRequest) *searchRequest {
	r := &searchRequest{
		Q:           req.QueryWithSiteOperators(),
		Autocorrect: true,
	}
	if req.MaxResults > 0 {
		r.Num = req.MaxResults
	}
	r.Tbs = recencyToTbs(req.Recency)
	return r
}

func recencyToTbs(r web.Recency) string {
	switch r {
	case web.RecencyHour:
		return "qdr:h"
	case web.RecencyDay:
		return "qdr:d"
	case web.RecencyWeek:
		return "qdr:w"
	case web.RecencyMonth:
		return "qdr:m"
	case web.RecencyYear:
		return "qdr:y"
	}
	return ""
}

func (s *searchResponse) toSearchResponse() *web.SearchResponse {
	results := make([]*web.SearchResult, 0, len(s.Organic))
	for _, searchResult := range s.Organic {
		results = append(results, &web.SearchResult{
			Title:         searchResult.Title,
			URL:           searchResult.Link,
			Snippet:       searchResult.Snippet,
			PublishedTime: parseDate(searchResult.Date),
		})
	}
	return &web.SearchResponse{Query: s.SearchParameters.Q, Results: results}
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
