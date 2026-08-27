package httpreq

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
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

	maxRedirects      = 10
	maxRequestTimeout = 120_000
)

// Method is an HTTP method exposed by the tool contract.
type Method string

const (
	MethodGET    Method = http.MethodGet
	MethodHEAD   Method = http.MethodHead
	MethodPOST   Method = http.MethodPost
	MethodPUT    Method = http.MethodPut
	MethodPATCH  Method = http.MethodPatch
	MethodDELETE Method = http.MethodDelete
)

// Resolve normalizes the method and applies GET when it is empty.
func (m Method) Resolve() Method {
	resolved := Method(strings.ToUpper(strings.TrimSpace(string(m))))
	if resolved == "" {
		return MethodGET
	}
	return resolved
}

// Validate reports whether the method is empty or supported by the tool.
func (m Method) Validate() error {
	switch m.Resolve() {
	case MethodGET, MethodHEAD, MethodPOST, MethodPUT, MethodPATCH, MethodDELETE:
		return nil
	default:
		return ErrInvalidMethod
	}
}

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
	AllowedMethods []Method

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

// Validate checks the network policy before a client is constructed.
func (c Config) Validate() error {
	if len(c.AllowedHosts) == 0 {
		return ErrMissingHosts
	}
	if _, err := NewAllowlist(c.AllowedHosts); err != nil {
		return fmt.Errorf("%w: AllowedHosts: %w", ErrInvalidConfig, err)
	}
	for _, method := range c.AllowedMethods {
		if strings.TrimSpace(string(method)) == "" {
			return fmt.Errorf("%w: AllowedMethods contains a blank method", ErrInvalidConfig)
		}
		if err := method.Validate(); err != nil {
			return fmt.Errorf("%w: AllowedMethods: %w", ErrInvalidConfig, err)
		}
	}
	if c.DefaultTimeout < 0 {
		return fmt.Errorf("%w: DefaultTimeout must not be negative", ErrInvalidConfig)
	}
	return nil
}

// Client executes HTTP requests through the configured allowlist.
type Client struct {
	http             *resty.Client
	allowedHosts     Allowlist
	allowedMethods   map[Method]struct{}
	maxResponseBytes int64
	defaultTimeout   time.Duration
}

func NewClient(cfg Config) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	allow, err := NewAllowlist(cfg.AllowedHosts)
	if err != nil {
		return nil, err
	}

	methods := cfg.AllowedMethods
	if len(methods) == 0 {
		methods = []Method{MethodGET, MethodHEAD}
	}
	allowedMethods := make(map[Method]struct{}, len(methods))
	for _, method := range methods {
		allowedMethods[method.Resolve()] = struct{}{}
	}

	var rc *resty.Client
	if cfg.HTTPClient != nil {
		// Resty's redirect policy mutates the underlying http.Client. Keep the
		// caller-owned client unchanged while retaining its transport, jar, and
		// other settings.
		httpClient := *cfg.HTTPClient
		rc = resty.NewWithClient(&httpClient)
	} else {
		rc = resty.New()
	}
	rc.SetRedirectPolicy(resty.RedirectPolicyFunc(allow.CheckRedirect))
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
		allowedHosts:     allow,
		allowedMethods:   allowedMethods,
		maxResponseBytes: maxBytes,
		defaultTimeout:   timeout,
	}, nil
}

// Request is the LLM-facing argument shape. The JSON / jsonschema tags
// drive the tool's input schema.
type Request struct {
	URL       string            `json:"url" jsonschema:"minLength=1" jsonschema_description:"Absolute http(s) URL. Host must match the configured allowlist."`
	Method    Method            `json:"method,omitempty" jsonschema:"enum=GET,enum=HEAD,enum=POST,enum=PUT,enum=PATCH,enum=DELETE" jsonschema_description:"HTTP method: GET (default), HEAD, POST, PUT, PATCH, or DELETE. Must be in the configured method allowlist."`
	Headers   map[string]string `json:"headers,omitempty" jsonschema_description:"Optional request headers. Values here override this tool's configured default headers."`
	Query     map[string]string `json:"query,omitempty" jsonschema_description:"Optional query parameters appended to the URL."`
	Body      string            `json:"body,omitempty" jsonschema_description:"Optional request body — for JSON, pass a JSON-encoded string and set Content-Type via Headers."`
	TimeoutMS int               `json:"timeout_ms,omitempty" jsonschema:"minimum=1,maximum=120000" jsonschema_description:"Per-call timeout in milliseconds, from 1 to 120000. Omit to use the configured default."`
}

// Prepare returns an independently owned, normalized request after validating
// it. The receiver and its header/query maps remain untouched.
func (r *Request) Prepare() (*Request, error) {
	if r == nil {
		return nil, ErrMissingRequest
	}
	prepared := *r
	prepared.URL = strings.TrimSpace(r.URL)
	prepared.Method = r.Method.Resolve()
	prepared.Headers = maps.Clone(r.Headers)
	prepared.Query = maps.Clone(r.Query)
	if err := prepared.Validate(); err != nil {
		return nil, err
	}
	return &prepared, nil
}

func (r *Request) Validate() error {
	if r == nil {
		return ErrMissingRequest
	}
	trimmedURL := strings.TrimSpace(r.URL)
	if trimmedURL == "" {
		return ErrEmptyURL
	}
	u, err := url.Parse(trimmedURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return ErrInvalidURL
	}
	if err := r.Method.Validate(); err != nil {
		return err
	}
	if r.TimeoutMS < 0 || r.TimeoutMS > maxRequestTimeout {
		return ErrInvalidTimeout
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
	prepared, err := req.Prepare()
	if err != nil {
		return nil, err
	}
	req = prepared

	method := req.Method.Resolve()
	if _, ok := c.allowedMethods[method]; !ok {
		return nil, fmt.Errorf("%w: %s", ErrMethodNotAllowed, method)
	}

	parsed, _ := url.Parse(req.URL)
	if !c.allowedHosts.Allows(parsed.Hostname()) {
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
	resp, err := r.Execute(string(method), req.URL)
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

// Allowlist matches hostnames against a set of exact + leading-wildcard
// patterns ("api.example.com", "*.example.com"). It is the host half of the
// tool's network policy and is exported for clients that need to inspect or
// share that exact policy. The zero value allows nothing.
type Allowlist struct{ patterns []hostPattern }

// NewAllowlist compiles host patterns. Each is an exact host or a single
// leading-'*.' wildcard; any other '*' is rejected.
func NewAllowlist(hosts []string) (Allowlist, error) {
	patterns := make([]hostPattern, 0, len(hosts))
	for _, h := range hosts {
		p, err := parseHostPattern(h)
		if err != nil {
			return Allowlist{}, err
		}
		patterns = append(patterns, p)
	}
	return Allowlist{patterns: patterns}, nil
}

// Empty reports whether the allowlist has no patterns (and so allows nothing).
func (a Allowlist) Empty() bool { return len(a.patterns) == 0 }

// Allows reports whether host matches any pattern (case-insensitive).
func (a Allowlist) Allows(host string) bool {
	host = strings.ToLower(host)
	for _, p := range a.patterns {
		if p.exact != "" && p.exact == host {
			return true
		}
		if p.suffix != "" && strings.HasSuffix(host, p.suffix) {
			return true
		}
	}
	return false
}

// CheckRedirect applies the allowlist to every redirect target and limits a
// request to the same number of redirects as net/http's default policy. It has
// the signature expected by [http.Client.CheckRedirect].
func (a Allowlist) CheckRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("httpreq: stopped after %d redirects", maxRedirects)
	}
	host := req.URL.Hostname()
	if !a.Allows(host) {
		return fmt.Errorf("%w: redirect target %q", ErrHostNotAllowed, host)
	}
	return nil
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
