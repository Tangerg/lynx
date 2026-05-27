package httpreq

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

const (
	// DefaultTimeout bounds a single request when the caller doesn't
	// override it.
	DefaultTimeout = 30 * time.Second

	// DefaultMaxResponseBytes caps the body size returned to the LLM
	// when [Config.MaxResponseBytes] is zero. 256 KiB is enough for
	// most JSON / text payloads without flooding the context window.
	DefaultMaxResponseBytes int64 = 256 * 1024
)

// safeMethods are the methods callers get by default — read-only ops
// only. Write methods (POST / PUT / PATCH / DELETE) must be opted-in
// explicitly via [Config.AllowedMethods].
var safeMethods = []string{http.MethodGet, http.MethodHead}

// Config controls the tool's network policy and resty client.
type Config struct {
	// AllowedHosts is the allowlist of permitted hosts. Required —
	// passing an empty slice is treated as a misconfiguration. Patterns
	// accept exact matches ("api.example.com") and one leading wildcard
	// ("*.example.com" matches "a.example.com" and "b.c.example.com",
	// but not "example.com" itself).
	AllowedHosts []string

	// AllowedMethods is the allowlist of permitted HTTP methods. When
	// nil/empty defaults to GET + HEAD. Comparison is case-insensitive.
	AllowedMethods []string

	// DefaultHeaders are added to every outgoing request unless the
	// per-call [Request.Headers] overrides them. Use this for
	// Authorization / User-Agent / API keys that shouldn't be at the
	// LLM's mercy.
	DefaultHeaders map[string]string

	// MaxResponseBytes caps the response body returned to the LLM.
	// 0 selects [DefaultMaxResponseBytes]; <0 disables the cap.
	MaxResponseBytes int64

	// DefaultTimeout bounds requests when [Request.TimeoutMS] is zero.
	// 0 selects [DefaultTimeout].
	DefaultTimeout time.Duration

	// HTTPClient lets callers swap the underlying http.Client (custom
	// transport, mTLS, proxy). Optional.
	HTTPClient *http.Client
}

func (c Config) Validate() error {
	if len(c.AllowedHosts) == 0 {
		return ErrMissingHosts
	}
	return nil
}

// Client executes HTTP requests through the configured allowlist.
type Client struct {
	http             *resty.Client
	allowedHosts     []hostPattern
	allowedMethods   map[string]struct{}
	maxResponseBytes int64
	defaultTimeout   time.Duration
}

func NewClient(cfg Config) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	patterns := make([]hostPattern, 0, len(cfg.AllowedHosts))
	for _, h := range cfg.AllowedHosts {
		p, err := parseHostPattern(h)
		if err != nil {
			return nil, err
		}
		patterns = append(patterns, p)
	}

	methods := cfg.AllowedMethods
	if len(methods) == 0 {
		methods = safeMethods
	}
	allowedMethods := make(map[string]struct{}, len(methods))
	for _, m := range methods {
		allowedMethods[strings.ToUpper(strings.TrimSpace(m))] = struct{}{}
	}

	var rc *resty.Client
	if cfg.HTTPClient != nil {
		rc = resty.NewWithClient(cfg.HTTPClient)
	} else {
		rc = resty.New()
	}
	for k, v := range cfg.DefaultHeaders {
		rc.SetHeader(k, v)
	}

	maxBytes := cfg.MaxResponseBytes
	if maxBytes == 0 {
		maxBytes = DefaultMaxResponseBytes
	}
	timeout := cfg.DefaultTimeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	return &Client{
		http:             rc,
		allowedHosts:     patterns,
		allowedMethods:   allowedMethods,
		maxResponseBytes: maxBytes,
		defaultTimeout:   timeout,
	}, nil
}

// Request is the LLM-facing argument shape. The JSON / jsonschema tags
// drive the tool's input schema.
type Request struct {
	URL       string            `json:"url" jsonschema:"required" jsonschema_description:"Absolute http(s) URL. Host must match the configured allowlist."`
	Method    string            `json:"method,omitempty" jsonschema_description:"HTTP method: GET (default) / HEAD / POST / PUT / PATCH / DELETE. Must be in the configured allowlist."`
	Headers   map[string]string `json:"headers,omitempty" jsonschema_description:"Optional request headers. Override any DefaultHeaders configured on the client."`
	Query     map[string]string `json:"query,omitempty" jsonschema_description:"Optional query parameters appended to the URL."`
	Body      string            `json:"body,omitempty" jsonschema_description:"Optional request body — for JSON, pass a JSON-encoded string and set Content-Type via Headers."`
	TimeoutMS int               `json:"timeout_ms,omitempty" jsonschema_description:"Optional per-call timeout in milliseconds. Typical: 5000-60000."`
}

func (r *Request) Validate() error {
	if r == nil {
		return ErrMissingRequest
	}
	if r.URL == "" {
		return ErrEmptyURL
	}
	u, err := url.Parse(r.URL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return ErrInvalidURL
	}
	return nil
}

// Response is the LLM-facing return shape. Body is always a string —
// every consumer is a chat model.
type Response struct {
	Status    int               `json:"status"`
	Headers   map[string]string `json:"headers,omitempty"`
	Body      string            `json:"body"`
	Truncated bool              `json:"truncated,omitempty"`
	Duration  string            `json:"duration"`
}

// Do executes req. The host + method are enforced against the
// configured allowlists; the response body is capped at
// [Config.MaxResponseBytes].
func (c *Client) Do(ctx context.Context, req *Request) (*Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = http.MethodGet
	}
	if _, ok := c.allowedMethods[method]; !ok {
		return nil, fmt.Errorf("%w: %s", ErrMethodNotAllowed, method)
	}

	parsed, _ := url.Parse(req.URL)
	if !c.hostAllowed(parsed.Hostname()) {
		return nil, fmt.Errorf("%w: %s", ErrHostNotAllowed, parsed.Hostname())
	}

	timeout := c.defaultTimeout
	if req.TimeoutMS > 0 {
		timeout = time.Duration(req.TimeoutMS) * time.Millisecond
	}

	r := c.http.R().
		SetContext(ctx).
		SetDoNotParseResponse(true)
	for k, v := range req.Headers {
		r.SetHeader(k, v)
	}
	for k, v := range req.Query {
		r.SetQueryParam(k, v)
	}
	if req.Body != "" {
		r.SetBody(req.Body)
	}

	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	r.SetContext(callCtx)

	start := time.Now()
	resp, err := r.Execute(method, req.URL)
	duration := time.Since(start)
	if err != nil {
		return nil, fmt.Errorf("httpreq: request failed: %w", err)
	}
	rawBody := resp.RawBody()
	defer rawBody.Close()

	body, truncated, err := readCapped(rawBody, c.maxResponseBytes)
	if err != nil {
		return nil, fmt.Errorf("httpreq: read body: %w", err)
	}

	return &Response{
		Status:    resp.StatusCode(),
		Headers:   flattenHeaders(resp.Header()),
		Body:      string(body),
		Truncated: truncated,
		Duration:  duration.String(),
	}, nil
}

// readCapped reads up to maxBytes from r. When maxBytes < 0 the cap is
// disabled and the whole body is read.
func readCapped(r io.Reader, maxBytes int64) ([]byte, bool, error) {
	if maxBytes < 0 {
		b, err := io.ReadAll(r)
		return b, false, err
	}
	limited := io.LimitReader(r, maxBytes+1)
	b, err := io.ReadAll(limited)
	if err != nil {
		return nil, false, err
	}
	if int64(len(b)) > maxBytes {
		return b[:maxBytes], true, nil
	}
	return b, false, nil
}

// flattenHeaders collapses http.Header (multi-valued) to map[string]string
// using comma-joining — what most LLMs expect to see.
func flattenHeaders(h http.Header) map[string]string {
	if len(h) == 0 {
		return nil
	}
	out := make(map[string]string, len(h))
	for k, v := range h {
		out[k] = strings.Join(v, ", ")
	}
	return out
}

// hostPattern is either an exact host or a leading-wildcard suffix.
type hostPattern struct {
	exact  string
	suffix string // "*.example.com" → ".example.com"
}

func parseHostPattern(s string) (hostPattern, error) {
	h := strings.ToLower(strings.TrimSpace(s))
	if h == "" {
		return hostPattern{}, errors.New("httpreq: empty host pattern in AllowedHosts")
	}
	if strings.HasPrefix(h, "*.") {
		return hostPattern{suffix: h[1:]}, nil
	}
	if strings.Contains(h, "*") {
		return hostPattern{}, fmt.Errorf("httpreq: host pattern %q — only leading '*.' wildcard is supported", s)
	}
	return hostPattern{exact: h}, nil
}

func (c *Client) hostAllowed(host string) bool {
	host = strings.ToLower(host)
	for _, p := range c.allowedHosts {
		if p.exact != "" && p.exact == host {
			return true
		}
		if p.suffix != "" && strings.HasSuffix(host, p.suffix) {
			return true
		}
	}
	return false
}
