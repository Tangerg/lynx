package firecrawl

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-resty/resty/v2"

	"github.com/Tangerg/lynx/tools/webfetch"
)

const (
	Name    = "firecrawl"
	baseURL = "https://api.firecrawl.dev/v2"
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

// NewClient returns a Firecrawl-backed client.
func NewClient(cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("firecrawl: APIKey is required")
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

type formatEntry struct {
	Type string `json:"type"`
}

type request struct {
	URL             string        `json:"url"`
	Formats         []formatEntry `json:"formats"`
	OnlyMainContent bool          `json:"onlyMainContent"`
}

func (r *request) validate() error {
	if r == nil {
		return errors.New("firecrawl: Request must not be nil")
	}
	if r.URL == "" {
		return errors.New("firecrawl: URL must not be empty")
	}
	return nil
}

type responseData struct {
	Markdown string  `json:"markdown,omitempty"`
	HTML     *string `json:"html,omitempty"`
}

type response struct {
	Success bool         `json:"success"`
	Data    responseData `json:"data"`
}

func (c *Client) fetch(ctx context.Context, req *request) (*response, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}
	var raw response
	resp, err := c.http.R().SetContext(ctx).SetBody(req).SetResult(&raw).Post("/scrape")
	if err != nil {
		return nil, fmt.Errorf("firecrawl: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("firecrawl: API error (status %d): %s", resp.StatusCode(), resp.String())
	}
	if !raw.Success {
		return nil, fmt.Errorf("firecrawl: scrape failed: %s", resp.String())
	}
	return &raw, nil
}

func (c *Client) Fetch(ctx context.Context, req *webfetch.Request) (*webfetch.Response, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("firecrawl: %w", err)
	}
	format := req.Format.Resolve()
	if format == webfetch.FormatText {
		format = webfetch.FormatMarkdown
	}
	raw, err := c.fetch(ctx, &request{
		URL:             req.URL,
		Formats:         []formatEntry{{Type: string(format)}},
		OnlyMainContent: true,
	})
	if err != nil {
		return nil, err
	}
	content := raw.Data.Markdown
	if format == webfetch.FormatHTML && raw.Data.HTML != nil {
		content = *raw.Data.HTML
	}
	return &webfetch.Response{Content: content, Format: format}, nil
}
