package jina

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-resty/resty/v2"

	"github.com/Tangerg/lynx/tools/webfetch"
)

const (
	Name    = "jina"
	baseURL = "https://r.jina.ai"
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

// NewClient returns a Jina Reader-backed client.
func NewClient(cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("jina: APIKey is required")
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
		SetHeader("Content-Type", "application/json").
		SetHeader("Accept", "application/json")}, nil
}

func (c *Client) Name() string { return Name }

type request struct {
	URL          string `json:"url"`
	ReturnFormat string `json:"-"`
}

func (r *request) validate() error {
	if r == nil {
		return errors.New("jina: Request must not be nil")
	}
	if r.URL == "" {
		return errors.New("jina: URL must not be empty")
	}
	return nil
}

type responseData struct {
	Content string `json:"content"`
}

type response struct {
	Data responseData `json:"data"`
}

func (c *Client) fetch(ctx context.Context, req *request) (*response, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}
	request := c.http.R().SetContext(ctx).
		SetBody(map[string]string{"url": req.URL}).
		SetHeader("X-Retain-Images", "none")
	if req.ReturnFormat != "" {
		request.SetHeader("X-Return-Format", req.ReturnFormat)
	}

	var raw response
	resp, err := request.SetResult(&raw).Post("/")
	if err != nil {
		return nil, fmt.Errorf("jina: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("jina: API error (status %d): %s", resp.StatusCode(), resp.String())
	}
	return &raw, nil
}

func (c *Client) Fetch(ctx context.Context, req *webfetch.Request) (*webfetch.Response, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("jina: %w", err)
	}
	format := req.Format.Resolve()
	raw, err := c.fetch(ctx, &request{URL: req.URL, ReturnFormat: string(format)})
	if err != nil {
		return nil, err
	}
	return &webfetch.Response{Content: raw.Data.Content, Format: format}, nil
}
