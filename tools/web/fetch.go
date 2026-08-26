package web

import (
	"context"
	"net/url"
	"strings"
)

// ContentFormat selects the representation of fetched page content.
// Providers map this to their native format setting.
type ContentFormat string

const (
	// FormatMarkdown returns the page rendered to Markdown. This is
	// the default and usually the most LLM-friendly format.
	FormatMarkdown ContentFormat = "markdown"
	// FormatHTML returns the page's HTML (or a cleaned variant).
	FormatHTML ContentFormat = "html"
	// FormatText returns plain text — no markup, no structure.
	FormatText ContentFormat = "text"
)

// Validate reports whether the format is empty or supported.
func (f ContentFormat) Validate() error {
	switch f {
	case "", FormatMarkdown, FormatHTML, FormatText:
		return nil
	default:
		return ErrInvalidFormat
	}
}

// Resolve applies the default format.
func (f ContentFormat) Resolve() ContentFormat {
	if f == "" {
		return FormatMarkdown
	}
	return f
}

// FetchRequest is both the provider-neutral fetch contract and the
// LLM-facing argument shape.
type FetchRequest struct {
	// URL is the page to fetch. Required.
	URL string `json:"url" jsonschema:"minLength=1" jsonschema_description:"Absolute http(s) URL of the page to fetch."`

	// Format selects the response format. "" defaults to markdown.
	Format ContentFormat `json:"format,omitempty" jsonschema:"enum=markdown,enum=html,enum=text" jsonschema_description:"Content format: markdown (default and best for readable structure), html, or text."`
}

// Validate checks that the request carries enough to act on. Returns
// one of the sentinel errors in errors.go so callers can match with
// errors.Is.
func (r *FetchRequest) Validate() error {
	if r == nil {
		return ErrMissingFetchRequest
	}
	r.URL = strings.TrimSpace(r.URL)
	if r.URL == "" {
		return ErrEmptyURL
	}
	parsed, err := url.Parse(r.URL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ErrInvalidURL
	}
	return r.Format.Validate()
}

// FetchResponse is the normalized scrape result. Used as both the SPI
// return type and the LLM-facing serialization shape.
type FetchResponse struct {
	Content string        `json:"content"`
	Format  ContentFormat `json:"format"`
}

// Fetcher is the SPI a page-fetching backend implements.
type Fetcher interface {
	// Fetch retrieves and renders the page at req.URL.
	Fetch(ctx context.Context, req *FetchRequest) (*FetchResponse, error)
}
