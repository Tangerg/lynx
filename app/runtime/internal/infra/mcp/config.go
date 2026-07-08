package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
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
		transport := &sdkmcp.StreamableClientTransport{
			Endpoint:     cfg.Endpoint,
			HTTPClient:   headerHTTPClient(cfg.Authorization, cfg.Headers),
			OAuthHandler: cfg.OAuthHandler,
		}
		return client.Connect(ctx, transport, nil)
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

func headerHTTPClient(authorization string, headers map[string]string) *http.Client {
	if authorization == "" && len(headers) == 0 {
		return nil
	}
	return &http.Client{Transport: &headerRoundTripper{
		authorization: authorization,
		headers:       headers,
		base:          http.DefaultTransport,
	}}
}

type headerRoundTripper struct {
	authorization string
	headers       map[string]string
	base          http.RoundTripper
}

func (t *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	for k, v := range t.headers {
		r.Header.Set(k, v)
	}
	if t.authorization != "" {
		r.Header.Set("Authorization", t.authorization)
	}
	return t.base.RoundTrip(r)
}
