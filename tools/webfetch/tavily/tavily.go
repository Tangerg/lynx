package tavily

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-resty/resty/v2"

	"github.com/Tangerg/lynx/tools/webfetch"
)

const (
	Name    = "tavily"
	baseURL = "https://api.tavily.com"
)

// Config configures [NewClient].
type Config struct {
	APIKey string
}

type Client struct {
	http *resty.Client
}

var _ webfetch.Provider = (*Client)(nil)

// NewClient returns a Tavily Extract-backed client.
func NewClient(cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("tavily: APIKey is required")
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

// Request is the full Tavily /extract request shape. The endpoint
// accepts multiple URLs per call; build with a single-element slice
// for the most common case.
type Request struct {
	// URLs accepts a single URL or a list (up to 20).
	URLs []string `json:"urls"`

	// Query, when set, reranks extracted chunks by relevance and
	// joins the top chunks with "[...]" separators.
	Query string `json:"query,omitempty"`

	// ChunksPerSource caps relevant chunks per URL (1-5). Only used
	// when Query is set.
	ChunksPerSource int `json:"chunks_per_source,omitempty"`

	// ExtractDepth: "basic" (default, 1 credit / 5 URLs) or
	// "advanced" (2 credits / 5 URLs, captures tables and embeds).
	ExtractDepth string `json:"extract_depth,omitempty"`

	// IncludeImages returns image URLs found on the page.
	IncludeImages bool `json:"include_images,omitempty"`

	// IncludeFavicon returns the favicon URL.
	IncludeFavicon bool `json:"include_favicon,omitempty"`

	// Format: "markdown" (default) or "text". HTML is not supported.
	Format string `json:"format,omitempty"`

	// Timeout caps each URL extraction in seconds (1.0-60.0).
	Timeout float64 `json:"timeout,omitempty"`

	// IncludeUsage returns credit accounting in the response.
	IncludeUsage bool `json:"include_usage,omitempty"`
}

func (r *Request) Validate() error {
	if r == nil {
		return errors.New("tavily: Request must not be nil")
	}
	if len(r.URLs) == 0 {
		return errors.New("tavily: URLs must not be empty")
	}
	return nil
}

// Result is one entry in [Response.Results].
type Result struct {
	URL        string   `json:"url"`
	RawContent string   `json:"raw_content"`
	Images     []string `json:"images,omitempty"`
	Favicon    string   `json:"favicon,omitempty"`
}

// FailedResult describes a URL Tavily couldn't extract.
type FailedResult struct {
	URL   string `json:"url"`
	Error string `json:"error"`
}

// Usage echoes credit accounting when IncludeUsage is true.
type Usage struct {
	Credits int `json:"credits"`
}

// Response is the full Tavily /extract response.
type Response struct {
	Results       []*Result       `json:"results"`
	FailedResults []*FailedResult `json:"failed_results"`
	ResponseTime  float64         `json:"response_time"`
	Usage         *Usage          `json:"usage,omitempty"`
	RequestID     string          `json:"request_id,omitempty"`
}

// FetchNative calls POST /extract with the full Tavily request shape.
func (c *Client) FetchNative(ctx context.Context, req *Request) (*Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	var raw Response
	resp, err := c.http.R().SetContext(ctx).SetBody(req).SetResult(&raw).Post("/extract")
	if err != nil {
		return nil, fmt.Errorf("tavily: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("tavily: API error (status %d): %s", resp.StatusCode(), resp.String())
	}
	return &raw, nil
}

// ============================================================== SPI wrapper

func (c *Client) Fetch(ctx context.Context, req *webfetch.Request) (*webfetch.Response, error) {
	format := req.ResolvedFormat()
	// Tavily Extract's format enum is markdown|text only; HTML maps
	// to markdown (closest clean rendering).
	wireFormat := format
	if wireFormat == webfetch.FormatHTML {
		wireFormat = webfetch.FormatMarkdown
	}

	raw, err := c.FetchNative(ctx, &Request{
		URLs:         []string{req.URL},
		ExtractDepth: "basic",
		Format:       string(wireFormat),
	})
	if err != nil {
		return nil, err
	}
	if len(raw.Results) == 0 {
		if len(raw.FailedResults) > 0 {
			return nil, fmt.Errorf("tavily: extract failed: %s", raw.FailedResults[0].Error)
		}
		return nil, fmt.Errorf("tavily: empty result for %s", req.URL)
	}
	return &webfetch.Response{Content: raw.Results[0].RawContent, Format: wireFormat}, nil
}
