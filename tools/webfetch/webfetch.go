package webfetch

import "context"

// ResponseFormat selects the format of the scraped page content.
// Providers map this to their native format setting.
type ResponseFormat string

const (
	// FormatMarkdown returns the page rendered to Markdown. This is
	// the default and usually the most LLM-friendly format.
	FormatMarkdown ResponseFormat = "markdown"
	// FormatHTML returns the page's HTML (or a cleaned variant).
	FormatHTML ResponseFormat = "html"
	// FormatText returns plain text — no markup, no structure.
	FormatText ResponseFormat = "text"
)

// Request is the shape every [Provider] consumes AND the LLM-facing
// argument shape — the two were identical so they're one type now.
// The JSON / jsonschema tags drive [Tool]'s input schema; provider
// impls read the Go fields directly.
type Request struct {
	// URL is the page to fetch. Required.
	URL string `json:"url" jsonschema:"required" jsonschema_description:"Absolute http(s) URL of the page to fetch."`

	// Format selects the response format. "" defaults to markdown.
	Format ResponseFormat `json:"format,omitempty" jsonschema_description:"Response format: \"markdown\" (default — best for LLMs), \"html\", or \"text\"."`
}

// Validate checks that the request carries enough to act on. Returns
// one of the sentinel errors in errors.go so callers can match with
// errors.Is.
func (r *Request) Validate() error {
	if r == nil {
		return ErrMissingRequest
	}
	if r.URL == "" {
		return ErrEmptyURL
	}
	return nil
}

// Response is the normalized scrape result. Used as both the SPI
// return type and the LLM-facing serialization shape.
type Response struct {
	Content string         `json:"content"`
	Format  ResponseFormat `json:"format"`
}

// Provider is the SPI a scrape backend implements.
type Provider interface {
	// Name returns the provider's identifier (e.g. "jina", "firecrawl").
	Name() string

	// Fetch retrieves and renders the page at req.URL.
	Fetch(ctx context.Context, req *Request) (*Response, error)
}
