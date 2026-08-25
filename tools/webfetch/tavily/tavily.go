package tavily

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-resty/resty/v2"

	"github.com/Tangerg/lynx/tools/webfetch"
)

const (
	Name    = "tavily"
	baseURL = "https://api.tavily.com"
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

var _ webfetch.Provider = (*Client)(nil)

// NewClient returns a Tavily Extract-backed client.
func NewClient(cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("tavily: APIKey is required")
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = baseURL
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{}
	}
	return &Client{http: resty.NewWithClient(cfg.HTTPClient).
		SetBaseURL(cfg.BaseURL).
		SetAuthToken(cfg.APIKey).
		SetHeader("Content-Type", "application/json")}, nil
}

func (c *Client) Name() string { return Name }

type request struct {
	URLs         []string `json:"urls"`
	ExtractDepth string   `json:"extract_depth,omitempty"`
	Format       string   `json:"format,omitempty"`
}

func (r *request) validate() error {
	if r == nil {
		return errors.New("tavily: Request must not be nil")
	}
	if len(r.URLs) == 0 {
		return errors.New("tavily: URLs must not be empty")
	}
	return nil
}

type result struct {
	RawContent string `json:"raw_content"`
}

type failedResult struct {
	Error string `json:"error"`
}

type response struct {
	Results       []*result       `json:"results"`
	FailedResults []*failedResult `json:"failed_results"`
}

func (c *Client) fetch(ctx context.Context, req *request) (*response, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}
	var raw response
	resp, err := c.http.R().SetContext(ctx).SetBody(req).SetResult(&raw).Post("/extract")
	if err != nil {
		return nil, fmt.Errorf("tavily: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("tavily: API error (status %d): %s", resp.StatusCode(), resp.String())
	}
	return &raw, nil
}

func (c *Client) Fetch(ctx context.Context, req *webfetch.Request) (*webfetch.Response, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("tavily: %w", err)
	}
	format := req.Format.Resolve()
	if format == webfetch.FormatHTML {
		format = webfetch.FormatMarkdown
	}
	raw, err := c.fetch(ctx, &request{
		URLs:         []string{req.URL},
		ExtractDepth: "basic",
		Format:       string(format),
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
	return &webfetch.Response{Content: raw.Results[0].RawContent, Format: format}, nil
}
