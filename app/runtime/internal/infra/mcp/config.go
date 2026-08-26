package mcp

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"os"
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
type Transport string

const (
	// TransportHTTP is Streamable HTTP. [ServerConfig.Endpoint] is the URL.
	TransportHTTP Transport = "http"
	// TransportStdio is a local subprocess over stdin/stdout.
	TransportStdio Transport = "stdio"
)

// Valid reports whether transport names one supported MCP connection mode.
func (t Transport) Valid() bool {
	return t == TransportHTTP || t == TransportStdio
}

func (t Transport) String() string {
	if !t.Valid() {
		return "invalid"
	}
	return string(t)
}

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
func (s ServerConfig) Validate() error {
	if s.Name == "" {
		return errors.New("mcp: server name is required")
	}
	switch s.Transport {
	case TransportHTTP:
		if s.Endpoint == "" {
			return fmt.Errorf("mcp server %q: Endpoint is required for HTTP transport", s.Name)
		}
		if _, err := httporigin.Parse(s.Endpoint); err != nil {
			return fmt.Errorf("mcp server %q: invalid Endpoint: %w", s.Name, err)
		}
		if s.Command != "" {
			return fmt.Errorf("mcp server %q: Command must be empty for HTTP transport", s.Name)
		}
		if s.OAuthHandler != nil && s.hasStaticAuthorization() {
			return fmt.Errorf("mcp server %q: OAuth and static Authorization are mutually exclusive", s.Name)
		}
	case TransportStdio:
		if s.Command == "" {
			return fmt.Errorf("mcp server %q: Command is required for stdio transport", s.Name)
		}
		if s.Endpoint != "" {
			return fmt.Errorf("mcp server %q: Endpoint must be empty for stdio transport", s.Name)
		}
		if s.Authorization != "" {
			return fmt.Errorf("mcp server %q: Authorization applies to HTTP transport only", s.Name)
		}
		if len(s.Headers) > 0 {
			return fmt.Errorf("mcp server %q: Headers apply to HTTP transport only", s.Name)
		}
		if s.OAuthHandler != nil {
			return fmt.Errorf("mcp server %q: OAuth applies to HTTP transport only", s.Name)
		}
	default:
		return fmt.Errorf("mcp server %q: unknown transport %q", s.Name, s.Transport)
	}
	return nil
}

func (s ServerConfig) hasStaticAuthorization() bool {
	if s.Authorization != "" {
		return true
	}
	for name, value := range s.Headers {
		if strings.EqualFold(name, "Authorization") && value != "" {
			return true
		}
	}
	return false
}

func dial(
	ctx context.Context,
	lifetime context.Context,
	client *sdkmcp.Client,
	cfg ServerConfig,
) (*sdkmcp.ClientSession, sessionCleanup, error) {
	if ctx == nil {
		return nil, nil, errors.New("mcp: dial context is required")
	}
	if lifetime == nil {
		return nil, nil, errors.New("mcp: session lifetime is required")
	}
	if err := cfg.Validate(); err != nil {
		return nil, nil, err
	}
	if client == nil {
		return nil, nil, errors.New("mcp: client must not be nil")
	}
	var command *exec.Cmd
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
			prepareStdioProcess(cmd)
			command = cmd
			return client.Connect(sessionCtx, &sdkmcp.CommandTransport{Command: cmd}, nil)
		default:
			return nil, fmt.Errorf("mcp: unknown transport %q", cfg.Transport)
		}
	}
	session, cancelLifetime, err := connectSession(ctx, lifetime, cfg.Timeout, connect)
	cleanup := sessionCleanup(func() error {
		if cancelLifetime != nil {
			cancelLifetime()
		}
		if command == nil {
			return nil
		}
		stopStdioProcessErr := stopStdioProcess(command)
		if errors.Is(stopStdioProcessErr, os.ErrProcessDone) {
			return nil
		}
		return stopStdioProcessErr
	})
	if err != nil {
		return nil, nil, errors.Join(err, cleanup())
	}
	return session, cleanup, nil
}

type sessionCleanup func() error

// connectSession gives an MCP session a lifecycle distinct from the operation
// that establishes it. Parent cancellation and the configured timeout still
// abort the handshake, but after Connect succeeds the session remains alive
// until its owner explicitly cancels it during detach, replacement, or
// shutdown. Binding the session directly to a short-lived command context
// makes a successful dynamic Configure look connected while its transport is
// already being torn down.
func connectSession(
	parent context.Context,
	lifetime context.Context,
	timeout time.Duration,
	connect func(context.Context) (*sdkmcp.ClientSession, error),
) (*sdkmcp.ClientSession, context.CancelFunc, error) {
	if parent == nil {
		return nil, nil, errors.New("mcp: handshake context is required")
	}
	if lifetime == nil {
		return nil, nil, errors.New("mcp: session lifetime is required")
	}
	lifetimeCtx, cancelLifetime := context.WithCancel(lifetime)
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

func (h *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := h.validateTarget(req.URL); err != nil {
		return nil, err
	}
	r := req.Clone(req.Context())
	for k, v := range h.headers {
		r.Header.Set(k, v)
	}
	if h.authorization != "" {
		r.Header.Set("Authorization", h.authorization)
	}
	response, err := h.base.RoundTrip(r)
	if response != nil {
		h.lastStatus.Store(int64(response.StatusCode))
	}
	return response, err
}

func (h *headerRoundTripper) classifyDialError(err error) error {
	if err == nil {
		return nil
	}
	if h.lastStatus.Load() == http.StatusUnauthorized {
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

func (h *headerRoundTripper) validateTarget(target *url.URL) error {
	origin, err := httporigin.FromURL(target)
	if err != nil {
		return fmt.Errorf("%w: %v", errCrossOrigin, err)
	}
	if origin != h.origin {
		return fmt.Errorf("%w: target origin %s differs from configured origin %s",
			errCrossOrigin, origin, h.origin)
	}
	return nil
}
