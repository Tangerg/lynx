package jina

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-resty/resty/v2"

	"github.com/Tangerg/lynx/tools/webfetch"
)

const (
	Name    = "jina"
	baseURL = "https://r.jina.ai"
)

// Config configures [NewClient].
type Config struct {
	APIKey string
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
	return &Client{
		http: resty.New().
			SetBaseURL(baseURL).
			SetAuthToken(cfg.APIKey).
			SetHeader("Content-Type", "application/json").
			SetHeader("Accept", "application/json"),
	}, nil
}

func (c *Client) Name() string { return Name }

// ============================================================== Native API

// RetainMode controls how images and links are kept in the rendered
// page. Pass empty string to inherit Jina's defaults.
type RetainMode string

const (
	RetainNone RetainMode = "none"
	RetainAll  RetainMode = "all"
	RetainAlt  RetainMode = "alt"  // images only
	RetainText RetainMode = "text" // links only
)

// Request is the full Jina Reader request. Body is just {url}; the
// rest of the knobs ride as HTTP headers.
type Request struct {
	// URL is the target page. Required.
	URL string `json:"url"`

	// ReturnFormat: markdown (default), html, text, screenshot, pageshot.
	ReturnFormat string `json:"-"`

	// RetainImages / RetainLinks shape how the page is rendered.
	RetainImages RetainMode `json:"-"`
	RetainLinks  RetainMode `json:"-"`

	// RespondWith picks the rendering engine: empty / "readerlm-v2".
	RespondWith string `json:"-"`

	// JSONSchema, when set, asks Jina to extract structured data
	// matching the schema instead of returning the raw page.
	JSONSchema map[string]any `json:"-"`

	// Instruction is a natural-language extraction directive.
	Instruction string `json:"-"`

	// WithGeneratedAlt asks Jina to caption images via LLM.
	WithGeneratedAlt bool `json:"-"`
}

func (r *Request) Validate() error {
	if r == nil {
		return errors.New("jina: Request must not be nil")
	}
	if r.URL == "" {
		return errors.New("jina: URL must not be empty")
	}
	return nil
}

// Usage echoes token consumption.
type Usage struct {
	Tokens int `json:"tokens"`
}

// ResponseData carries the extracted page content.
type ResponseData struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
	Content     string `json:"content"`
	Usage       *Usage `json:"usage,omitempty"`
}

// Response is the full Jina Reader response.
type Response struct {
	Code   int          `json:"code"`
	Status int          `json:"status"`
	Data   ResponseData `json:"data"`
}

// FetchNative calls POST https://r.jina.ai/ with the full Jina
// request shape. Most fields ride as HTTP headers, not body fields.
func (c *Client) FetchNative(ctx context.Context, req *Request) (*Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	r := c.http.R().SetContext(ctx).SetBody(map[string]string{"url": req.URL})

	if req.ReturnFormat != "" {
		r.SetHeader("X-Return-Format", req.ReturnFormat)
	}
	if req.RetainImages != "" {
		r.SetHeader("X-Retain-Images", string(req.RetainImages))
	}
	if req.RetainLinks != "" {
		r.SetHeader("X-Retain-Links", string(req.RetainLinks))
	}
	if req.RespondWith != "" {
		r.SetHeader("X-Respond-With", req.RespondWith)
	}
	if req.Instruction != "" {
		r.SetHeader("X-Instruction", req.Instruction)
	}
	if req.WithGeneratedAlt {
		r.SetHeader("X-With-Generated-Alt", "true")
	}
	// JSONSchema isn't header-friendly; if needed, callers can use
	// HTTPClient() and add it themselves.

	var raw Response
	resp, err := r.SetResult(&raw).Post("/")
	if err != nil {
		return nil, fmt.Errorf("jina: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("jina: API error (status %d): %s", resp.StatusCode(), resp.String())
	}
	return &raw, nil
}

// ============================================================== SPI wrapper

func (c *Client) Fetch(ctx context.Context, req *webfetch.Request) (*webfetch.Response, error) {
	format := req.ResolvedFormat()
	raw, err := c.FetchNative(ctx, &Request{
		URL:          req.URL,
		ReturnFormat: string(format),
		RetainImages: RetainNone,
	})
	if err != nil {
		return nil, err
	}
	return &webfetch.Response{Content: raw.Data.Content, Format: format}, nil
}
