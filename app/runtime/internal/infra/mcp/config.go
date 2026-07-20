package mcp

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Transport is the wire mode of an MCP server connection. The zero value is
// invalid so a misconfigured runtime entry fails validation instead of silently
// defaulting.
type Transport int

const (
	// TransportHTTP is Streamable HTTP. [ServerConfig.Endpoint] is the URL.
	TransportHTTP Transport = iota + 1
	// TransportStdio is a local subprocess over stdin/stdout.
	TransportStdio
)

// ServerConfig declaratively describes one runtime MCP server connection. This
// is application configuration, not part of the reusable mcp package: the
// protocol package exposes transports and sessions, while the runtime owns how
// persisted descriptors become live sessions.
type ServerConfig struct {
	// Name identifies the server for tool namespacing and status reporting.
	// Required.
	Name string

	// Transport picks the connection mode. Required.
	Transport Transport

	// Endpoint is the Streamable HTTP URL. Used with [TransportHTTP].
	Endpoint string

	// Command is the executable to spawn. Used with [TransportStdio].
	Command string

	// Args are command arguments. Used with [TransportStdio].
	Args []string

	// Env, when non-nil, replaces the subprocess environment.
	Env []string

	// Dir sets the subprocess working directory.
	Dir string

	// Authorization is sent as the HTTP Authorization header.
	Authorization string

	// Headers carries extra static HTTP headers.
	Headers map[string]string

	// Timeout bounds the MCP initialize handshake. It does not bound the live
	// session after connection.
	Timeout time.Duration

	// OAuthHandler authorizes an HTTP connection via OAuth 2.1. It is live
	// process state and must not be persisted.
	OAuthHandler auth.OAuthHandler
}

// Validate reports whether exactly one transport is fully specified and the
// other transport's fields are blank.
func (c ServerConfig) Validate() error {
	if c.Name == "" {
		return errors.New("mcp: server name is required")
	}
	switch c.Transport {
	case TransportHTTP:
		if c.Endpoint == "" {
			return fmt.Errorf("mcp server %q: Endpoint is required for HTTP transport", c.Name)
		}
		if _, err := parseHTTPOrigin(c.Endpoint); err != nil {
			return fmt.Errorf("mcp server %q: invalid Endpoint: %w", c.Name, err)
		}
		if c.Command != "" {
			return fmt.Errorf("mcp server %q: Command must be empty for HTTP transport", c.Name)
		}
	case TransportStdio:
		if c.Command == "" {
			return fmt.Errorf("mcp server %q: Command is required for stdio transport", c.Name)
		}
		if c.Endpoint != "" {
			return fmt.Errorf("mcp server %q: Endpoint must be empty for stdio transport", c.Name)
		}
		if c.Authorization != "" {
			return fmt.Errorf("mcp server %q: Authorization applies to HTTP transport only", c.Name)
		}
		if len(c.Headers) > 0 {
			return fmt.Errorf("mcp server %q: Headers apply to HTTP transport only", c.Name)
		}
		if c.OAuthHandler != nil {
			return fmt.Errorf("mcp server %q: OAuth applies to HTTP transport only", c.Name)
		}
	default:
		return fmt.Errorf("mcp server %q: unknown transport %d", c.Name, c.Transport)
	}
	return nil
}

func dial(ctx context.Context, client *sdkmcp.Client, cfg ServerConfig) (*sdkmcp.ClientSession, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if client == nil {
		return nil, errors.New("mcp: client must not be nil")
	}
	if cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
	}
	switch cfg.Transport {
	case TransportHTTP:
		httpClient, err := endpointHTTPClient(cfg.Endpoint, cfg.Authorization, cfg.Headers)
		if err != nil {
			return nil, fmt.Errorf("mcp server %q: build HTTP client: %w", cfg.Name, err)
		}
		transport := &sdkmcp.StreamableClientTransport{
			Endpoint:     cfg.Endpoint,
			HTTPClient:   httpClient,
			OAuthHandler: cfg.OAuthHandler,
		}
		session, err := client.Connect(ctx, transport, nil)
		return session, classifyHTTPDialError(httpClient, err)
	case TransportStdio:
		cmd := exec.CommandContext(ctx, cfg.Command, cfg.Args...)
		if cfg.Env != nil {
			cmd.Env = cfg.Env
		}
		if cfg.Dir != "" {
			cmd.Dir = cfg.Dir
		}
		return client.Connect(ctx, &sdkmcp.CommandTransport{Command: cmd}, nil)
	default:
		return nil, fmt.Errorf("mcp: unknown transport %d", cfg.Transport)
	}
}

const maxRedirects = 10

var errCrossOrigin = errors.New("mcp: cross-origin request blocked")

type httpOrigin struct {
	scheme string
	host   string
}

func parseHTTPOrigin(rawURL string) (httpOrigin, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return httpOrigin{}, fmt.Errorf("parse URL: %w", err)
	}
	return originFromURL(u)
}

func originFromURL(u *url.URL) (httpOrigin, error) {
	if u == nil {
		return httpOrigin{}, errors.New("URL is nil")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return httpOrigin{}, fmt.Errorf("scheme %q is not HTTP or HTTPS", u.Scheme)
	}
	hostname := strings.ToLower(u.Hostname())
	if hostname == "" {
		return httpOrigin{}, errors.New("host is required")
	}
	port := u.Port()
	if port == "" {
		if scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return httpOrigin{scheme: scheme, host: net.JoinHostPort(hostname, port)}, nil
}

func endpointHTTPClient(endpoint, authorization string, headers map[string]string) (*http.Client, error) {
	origin, err := parseHTTPOrigin(endpoint)
	if err != nil {
		return nil, err
	}
	transport := &headerRoundTripper{
		origin:        origin,
		authorization: authorization,
		headers:       maps.Clone(headers),
		base:          http.DefaultTransport,
	}
	return &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("mcp: stopped after %d redirects", maxRedirects)
			}
			return transport.validateTarget(req.URL)
		},
	}, nil
}

type headerRoundTripper struct {
	origin        httpOrigin
	authorization string
	headers       map[string]string
	base          http.RoundTripper
	lastStatus    atomic.Int32
}

func (t *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := t.validateTarget(req.URL); err != nil {
		return nil, err
	}
	r := req.Clone(req.Context())
	for k, v := range t.headers {
		r.Header.Set(k, v)
	}
	if t.authorization != "" {
		r.Header.Set("Authorization", t.authorization)
	}
	response, err := t.base.RoundTrip(r)
	if response != nil {
		t.lastStatus.Store(int32(response.StatusCode))
	}
	return response, err
}

func (t *headerRoundTripper) classifyDialError(err error) error {
	if err == nil {
		return nil
	}
	if t.lastStatus.Load() == http.StatusUnauthorized {
		return &dialFailure{kind: dialFailureNeedsAuth, err: err}
	}
	return err
}

func classifyHTTPDialError(client *http.Client, err error) error {
	if transport, ok := client.Transport.(*headerRoundTripper); ok {
		return transport.classifyDialError(err)
	}
	return err
}

func (t *headerRoundTripper) validateTarget(target *url.URL) error {
	origin, err := originFromURL(target)
	if err != nil {
		return fmt.Errorf("%w: %v", errCrossOrigin, err)
	}
	if origin != t.origin {
		return fmt.Errorf("%w: target origin %s://%s differs from configured origin %s://%s",
			errCrossOrigin, origin.scheme, origin.host, t.origin.scheme, t.origin.host)
	}
	return nil
}
