package mcp

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Tangerg/lynx/app/runtime/internal/httporigin"
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

	// OAuthHandler authorizes an HTTP connection via OAuth 2.1. The handler is
	// live process state; its opaque session may be persisted separately through
	// [OAuthSessionStore].
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
		if _, err := httporigin.Parse(c.Endpoint); err != nil {
			return fmt.Errorf("mcp server %q: invalid Endpoint: %w", c.Name, err)
		}
		if c.Command != "" {
			return fmt.Errorf("mcp server %q: Command must be empty for HTTP transport", c.Name)
		}
		if c.OAuthHandler != nil && c.hasStaticAuthorization() {
			return fmt.Errorf("mcp server %q: OAuth and static Authorization are mutually exclusive", c.Name)
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

func (c ServerConfig) hasStaticAuthorization() bool {
	if c.Authorization != "" {
		return true
	}
	for name, value := range c.Headers {
		if strings.EqualFold(name, "Authorization") && value != "" {
			return true
		}
	}
	return false
}

func dial(
	ctx context.Context,
	client *sdkmcp.Client,
	cfg ServerConfig,
) (*sdkmcp.ClientSession, context.CancelFunc, error) {
	if err := cfg.Validate(); err != nil {
		return nil, nil, err
	}
	if client == nil {
		return nil, nil, errors.New("mcp: client must not be nil")
	}
	connect := func(sessionCtx context.Context) (*sdkmcp.ClientSession, error) {
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
			session, err := client.Connect(sessionCtx, transport, nil)
			return session, classifyHTTPDialError(httpClient, err)
		case TransportStdio:
			cmd := exec.CommandContext(sessionCtx, cfg.Command, cfg.Args...)
			if cfg.Env != nil {
				cmd.Env = cfg.Env
			}
			if cfg.Dir != "" {
				cmd.Dir = cfg.Dir
			}
			return client.Connect(sessionCtx, &sdkmcp.CommandTransport{Command: cmd}, nil)
		default:
			return nil, fmt.Errorf("mcp: unknown transport %d", cfg.Transport)
		}
	}
	return connectSession(ctx, cfg.Timeout, connect)
}

// connectSession gives an MCP session a lifecycle distinct from the operation
// that establishes it. Parent cancellation and the configured timeout still
// abort the handshake, but after Connect succeeds the session remains alive
// until its owner explicitly cancels it during detach, replacement, or
// shutdown. Binding the session directly to a short-lived command context
// makes a successful dynamic Configure look connected while its transport is
// already being torn down.
func connectSession(
	parent context.Context,
	timeout time.Duration,
	connect func(context.Context) (*sdkmcp.ClientSession, error),
) (*sdkmcp.ClientSession, context.CancelFunc, error) {
	if parent == nil {
		parent = context.Background()
	}
	lifetimeCtx, cancelLifetime := context.WithCancel(context.WithoutCancel(parent))
	handshakeCtx := parent
	cancelHandshake := func() {}
	if timeout > 0 {
		handshakeCtx, cancelHandshake = context.WithTimeout(parent, timeout)
	}
	stopHandshake := context.AfterFunc(handshakeCtx, cancelLifetime)

	session, err := connect(lifetimeCtx)
	if err != nil {
		stopHandshake()
		cancelHandshake()
		cancelLifetime()
		return nil, nil, err
	}
	if handshakeErr := handshakeCtx.Err(); handshakeErr != nil || !stopHandshake() {
		if handshakeErr == nil {
			handshakeErr = context.Canceled
		}
		cancelHandshake()
		cancelLifetime()
		return nil, nil, errors.Join(handshakeErr, session.Close())
	}
	cancelHandshake()
	return session, cancelLifetime, nil
}

const maxRedirects = 10

var errCrossOrigin = errors.New("mcp: cross-origin request blocked")

func endpointHTTPClient(endpoint, authorization string, headers map[string]string) (*http.Client, error) {
	origin, err := httporigin.Parse(endpoint)
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
	origin        httporigin.Origin
	authorization string
	headers       map[string]string
	base          http.RoundTripper
	lastStatus    atomic.Int64
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
		t.lastStatus.Store(int64(response.StatusCode))
	}
	return response, err
}

func (t *headerRoundTripper) classifyDialError(err error) error {
	if err == nil {
		return nil
	}
	if t.lastStatus.Load() == http.StatusUnauthorized {
		return &dialError{kind: dialErrorNeedsAuth, err: err}
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
	origin, err := httporigin.FromURL(target)
	if err != nil {
		return fmt.Errorf("%w: %v", errCrossOrigin, err)
	}
	if origin != t.origin {
		return fmt.Errorf("%w: target origin %s differs from configured origin %s",
			errCrossOrigin, origin, t.origin)
	}
	return nil
}
