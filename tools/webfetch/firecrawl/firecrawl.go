package firecrawl

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-resty/resty/v2"

	"github.com/Tangerg/lynx/tools/webfetch"
)

const (
	Name    = "firecrawl"
	baseURL = "https://api.firecrawl.dev/v2"
)

// Config configures [NewClient].
type Config struct {
	APIKey string
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
	return &Client{
		http: resty.New().
			SetBaseURL(baseURL).
			SetAuthToken(cfg.APIKey).
			SetHeader("Content-Type", "application/json"),
	}, nil
}

func (c *Client) Name() string { return Name }

// ============================================================== Native API

// FormatEntry is one entry in [Request.Formats]. Firecrawl accepts
// both bare strings and richer objects ({type, schema}); this struct
// only models the {type} form. Pass a different shape via Extras when
// you need json or summary modes.
type FormatEntry struct {
	Type string `json:"type"`
}

// ProxyType selects which proxy tier Firecrawl uses.
type ProxyType string

const (
	ProxyBasic    ProxyType = "basic"
	ProxyEnhanced ProxyType = "enhanced"
	ProxyAuto     ProxyType = "auto"
)

// PDFParserMode controls PDF parsing.
type PDFParserMode string

const (
	PDFParserFast PDFParserMode = "fast"
	PDFParserAuto PDFParserMode = "auto"
	PDFParserOCR  PDFParserMode = "ocr"
)

// PDFParser configures PDF parsing.
type PDFParser struct {
	Type     string        `json:"type"` // "pdf"
	Mode     PDFParserMode `json:"mode,omitempty"`
	MaxPages int           `json:"maxPages,omitempty"`
}

// LocationOptions sets geo-proxy and locale emulation.
type LocationOptions struct {
	Country   string   `json:"country,omitempty"`
	Languages []string `json:"languages,omitempty"`
}

// ViewportOptions sets the browser viewport dimensions.
type ViewportOptions struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Action is the marker interface implemented by every action type.
// Use the concrete structs (ActionWait, ActionClick, ...) directly.
type Action interface {
	actionMarker()
}

// ActionWait pauses for a duration or until a selector appears.
type ActionWait struct {
	Type         string `json:"type"` // "wait"
	Milliseconds int    `json:"milliseconds,omitempty"`
	Selector     string `json:"selector,omitempty"`
}

func (ActionWait) actionMarker() {}

// ActionScreenshot captures the current page state.
type ActionScreenshot struct {
	Type     string           `json:"type"` // "screenshot"
	FullPage bool             `json:"fullPage,omitempty"`
	Quality  int              `json:"quality,omitempty"`
	Viewport *ViewportOptions `json:"viewport,omitempty"`
}

func (ActionScreenshot) actionMarker() {}

// ActionClick clicks element(s) matching a CSS selector.
type ActionClick struct {
	Type     string `json:"type"` // "click"
	Selector string `json:"selector"`
	All      bool   `json:"all,omitempty"`
}

func (ActionClick) actionMarker() {}

// ActionWrite types text into the currently focused element.
type ActionWrite struct {
	Type string `json:"type"` // "write"
	Text string `json:"text"`
}

func (ActionWrite) actionMarker() {}

// ActionPress sends a single key press.
type ActionPress struct {
	Type string `json:"type"` // "press"
	Key  string `json:"key"`
}

func (ActionPress) actionMarker() {}

// ActionScroll scrolls the page or an element.
type ActionScroll struct {
	Type      string `json:"type"` // "scroll"
	Direction string `json:"direction,omitempty"`
	Selector  string `json:"selector,omitempty"`
}

func (ActionScroll) actionMarker() {}

// ActionScrape captures URL and HTML mid-sequence.
type ActionScrape struct {
	Type string `json:"type"` // "scrape"
}

func (ActionScrape) actionMarker() {}

// ActionExecuteJavaScript runs arbitrary JS on the page.
type ActionExecuteJavaScript struct {
	Type   string `json:"type"` // "executeJavascript"
	Script string `json:"script"`
}

func (ActionExecuteJavaScript) actionMarker() {}

// ActionPDF generates a PDF of the current page.
type ActionPDF struct {
	Type      string  `json:"type"` // "pdf"
	Format    string  `json:"format,omitempty"`
	Landscape bool    `json:"landscape,omitempty"`
	Scale     float64 `json:"scale,omitempty"`
}

func (ActionPDF) actionMarker() {}

// Request is the full Firecrawl /v2/scrape request shape.
type Request struct {
	// URL is the page to scrape. Required.
	URL string `json:"url"`

	// Formats picks one or more output formats — the SPI wrapper wires the
	// resolved format as [{"type": "markdown"}] etc.; richer modes (json,
	// summary) need the {type, schema}-style objects.
	Formats []FormatEntry `json:"formats"`

	// OnlyMainContent strips boilerplate (defaults to true).
	OnlyMainContent bool `json:"onlyMainContent"`

	// IncludeTags / ExcludeTags filter the DOM before extraction.
	IncludeTags []string `json:"includeTags,omitempty"`
	ExcludeTags []string `json:"excludeTags,omitempty"`

	// MaxAge in milliseconds; serve cached version if fresh enough.
	MaxAge int `json:"maxAge,omitempty"`

	// Headers are extra HTTP headers sent to the target URL.
	Headers map[string]string `json:"headers,omitempty"`

	// WaitFor delays content capture in milliseconds.
	WaitFor int `json:"waitFor,omitempty"`

	// Mobile emulates a mobile viewport.
	Mobile bool `json:"mobile,omitempty"`

	// SkipTLSVerification disables TLS cert checks.
	SkipTLSVerification bool `json:"skipTlsVerification,omitempty"`

	// Timeout caps the scrape in milliseconds (max 300000).
	Timeout int `json:"timeout,omitempty"`

	// Parsers controls special-file handling (PDFs).
	Parsers []PDFParser `json:"parsers,omitempty"`

	// Actions runs browser-automation steps before extraction.
	Actions []Action `json:"actions,omitempty"`

	// Location sets geo-proxy / locale emulation.
	Location *LocationOptions `json:"location,omitempty"`

	// RemoveBase64Images strips inline base64 images (default true).
	RemoveBase64Images bool `json:"removeBase64Images,omitempty"`

	// BlockAds toggles ad / cookie-popup blocking (default true).
	BlockAds bool `json:"blockAds,omitempty"`

	// Proxy selects the proxy tier (default ProxyAuto).
	Proxy ProxyType `json:"proxy,omitempty"`

	// StoreInCache controls whether the result is cached.
	StoreInCache bool `json:"storeInCache,omitempty"`

	// ZeroDataRetention disables all data retention (enterprise).
	ZeroDataRetention bool `json:"zeroDataRetention,omitempty"`
}

func (r *Request) Validate() error {
	if r == nil {
		return errors.New("firecrawl: Request must not be nil")
	}
	if r.URL == "" {
		return errors.New("firecrawl: URL must not be empty")
	}
	return nil
}

// ActionScrapeResult captures one ActionScrape's output.
type ActionScrapeResult struct {
	URL  string `json:"url,omitempty"`
	HTML string `json:"html,omitempty"`
}

// JavaScriptReturn captures one ActionExecuteJavaScript's output.
type JavaScriptReturn struct {
	Type  string `json:"type,omitempty"`
	Value any    `json:"value,omitempty"`
}

// ActionsResult collects browser-action outputs in execution order.
type ActionsResult struct {
	Screenshots       []string             `json:"screenshots,omitempty"`
	Scrapes           []ActionScrapeResult `json:"scrapes,omitempty"`
	JavascriptReturns []JavaScriptReturn   `json:"javascriptReturns,omitempty"`
	PDFs              []string             `json:"pdfs,omitempty"`
}

// ResponseMetadata holds page-level metadata.
type ResponseMetadata struct {
	Title       any    `json:"title,omitempty"`
	Description any    `json:"description,omitempty"`
	Language    any    `json:"language,omitempty"`
	SourceURL   string `json:"sourceURL,omitempty"`
	URL         string `json:"url,omitempty"`
	StatusCode  int    `json:"statusCode,omitempty"`
	Error       string `json:"error,omitempty"`
}

// ResponseData carries the scrape output.
type ResponseData struct {
	Markdown   string            `json:"markdown,omitempty"`
	Summary    *string           `json:"summary,omitempty"`
	HTML       *string           `json:"html,omitempty"`
	RawHTML    *string           `json:"rawHtml,omitempty"`
	Screenshot *string           `json:"screenshot,omitempty"`
	Links      []string          `json:"links,omitempty"`
	Actions    *ActionsResult    `json:"actions,omitempty"`
	Metadata   *ResponseMetadata `json:"metadata,omitempty"`
	Warning    *string           `json:"warning,omitempty"`
}

// Response is the full Firecrawl /scrape response.
type Response struct {
	Success bool         `json:"success"`
	Data    ResponseData `json:"data"`
}

// FetchNative calls POST /scrape with the full Firecrawl request shape.
func (c *Client) FetchNative(ctx context.Context, req *Request) (*Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	var raw Response
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

// ============================================================== SPI wrapper

func (c *Client) Fetch(ctx context.Context, req *webfetch.Request) (*webfetch.Response, error) {
	format := req.ResolvedFormat()
	raw, err := c.FetchNative(ctx, &Request{
		URL:             req.URL,
		Formats:         []FormatEntry{{Type: string(format)}},
		OnlyMainContent: true,
	})
	if err != nil {
		return nil, err
	}
	return &webfetch.Response{Content: raw.Data.content(format), Format: format}, nil
}

// content selects the field matching the caller's requested format.
// Firecrawl returns multiple formats simultaneously, so the wrapper picks
// the one the caller asked for.
func (d ResponseData) content(format webfetch.ResponseFormat) string {
	switch format {
	case webfetch.FormatHTML:
		if d.HTML != nil {
			return *d.HTML
		}
	case webfetch.FormatText:
		if d.RawHTML != nil {
			return *d.RawHTML
		}
	}
	return d.Markdown
}
