package exa

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-resty/resty/v2"

	"github.com/Tangerg/lynx/tools/webfetch"
)

const (
	Name    = "exa"
	baseURL = "https://api.exa.ai"
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

// NewClient returns an Exa-backed client.
func NewClient(cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("exa: APIKey is required")
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
			SetHeader("x-api-key", cfg.APIKey).
			SetHeader("Content-Type", "application/json"),
	}, nil
}

func (c *Client) Name() string { return Name }

type textOptions struct {
	IncludeHTMLTags bool `json:"includeHtmlTags,omitempty"`
}

type request struct {
	URLs []string    `json:"urls,omitempty"`
	Text textOptions `json:"text,omitempty"`
}

func (r *request) validate() error {
	if r == nil {
		return errors.New("exa: Request must not be nil")
	}
	if len(r.URLs) == 0 {
		return errors.New("exa: URLs must be non-empty")
	}
	return nil
}

type result struct {
	Text string `json:"text,omitempty"`
}

type response struct {
	Results []*result `json:"results"`
}

func (c *Client) fetch(ctx context.Context, req *request) (*response, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}
	var raw response
	resp, err := c.http.R().SetContext(ctx).SetBody(req).SetResult(&raw).Post("/contents")
	if err != nil {
		return nil, fmt.Errorf("exa: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("exa: API error (status %d): %s", resp.StatusCode(), resp.String())
	}
	return &raw, nil
}

func (c *Client) Fetch(ctx context.Context, req *webfetch.Request) (*webfetch.Response, error) {
	format := req.ResolvedFormat()
	raw, err := c.fetch(ctx, &request{
		URLs: []string{req.URL},
		Text: textOptions{IncludeHTMLTags: format == webfetch.FormatHTML},
	})
	if err != nil {
		return nil, err
	}
	if len(raw.Results) == 0 {
		return nil, fmt.Errorf("exa: empty result for %s", req.URL)
	}
	return &webfetch.Response{Content: raw.Results[0].Text, Format: format}, nil
}
