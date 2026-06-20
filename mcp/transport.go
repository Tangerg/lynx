package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// HTTPServerOptions configures a Streamable HTTP server handler built
// by [NewStreamableHTTPHandler]. All fields are optional — the empty
// value is a sensible default for a single-tenant, stateful server.
type HTTPServerOptions struct {
	// GetServer maps an incoming HTTP request onto an [*sdkmcp.Server].
	// Multi-tenant deployments use this to dispatch by URL path /
	// header. When nil, the handler returns the single server passed
	// to [NewStreamableHTTPHandler] for every request.
	GetServer func(*http.Request) *sdkmcp.Server

	// Stateless skips the Mcp-Session-Id validation and uses an
	// ephemeral session per request. Trades server→client push for
	// horizontal-scaling simplicity.
	Stateless bool

	// JSONResponse switches stream responses from text/event-stream to
	// application/json (§2.1.5 of the MCP spec). Useful when fronted
	// by HTTP proxies that don't tolerate SSE.
	JSONResponse bool
}

// NewStreamableHTTPHandler exposes server as an [http.Handler] over
// the modern Streamable HTTP transport — the default since MCP spec
// 2025-03-26 (replaces the legacy two-endpoint SSE transport).
//
// Mount the returned handler on any standard library / framework
// router:
//
//	mux := http.NewServeMux()
//	mux.Handle("/mcp", mcp.NewStreamableHTTPHandler(server, mcp.HTTPServerOptions{}))
//	_ = http.ListenAndServe(":8080", mux)
//
// Multi-tenant routing example:
//
//	h := mcp.NewStreamableHTTPHandler(nil, mcp.HTTPServerOptions{
//	    GetServer: func(r *http.Request) *sdkmcp.Server {
//	        return registry[r.PathValue("tenant")]
//	    },
//	})
func NewStreamableHTTPHandler(server *sdkmcp.Server, opts HTTPServerOptions) http.Handler {
	getServer := opts.GetServer
	if getServer == nil {
		// Capture the server pointer so the closure is independent of
		// the option struct after this constructor returns.
		captured := server
		getServer = func(*http.Request) *sdkmcp.Server { return captured }
	}

	return sdkmcp.NewStreamableHTTPHandler(getServer, &sdkmcp.StreamableHTTPOptions{
		Stateless:    opts.Stateless,
		JSONResponse: opts.JSONResponse,
	})
}

// HTTPClientOptions configures a Streamable HTTP client connection
// built by [DialStreamableHTTP].
type HTTPClientOptions struct {
	// HTTPClient overrides the default [http.Client] — use it to plug
	// in custom transport (TLS, proxies, OTel instrumentation, ...).
	// Optional.
	HTTPClient *http.Client

	// MaxRetries is the maximum number of reconnect attempts after a
	// transient transport failure. Default 5; set to a negative
	// number to disable retries entirely.
	MaxRetries int

	// DisableStandaloneSSE skips opening the persistent GET stream
	// used for server-initiated notifications (per spec, this is
	// optional). Leave false unless the server rejects GET or the
	// client only does request/response.
	DisableStandaloneSSE bool
}

// DialStreamableHTTP connects client to a Streamable HTTP MCP server
// at endpoint and returns the initialized [*sdkmcp.ClientSession]. The
// caller is responsible for closing the session when done.
//
// Example — connect, list tools, close:
//
//	cli := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "agent", Version: "v0.1"}, nil)
//	session, err := mcp.DialStreamableHTTP(ctx, cli,
//	    "https://mcp.example.com/", mcp.HTTPClientOptions{})
//	if err != nil { return err }
//	defer session.Close()
//
//	res, err := session.ListTools(ctx, nil)
//	...
//
// Returns an error when client is nil or endpoint is empty.
func DialStreamableHTTP(
	ctx context.Context,
	client *sdkmcp.Client,
	endpoint string,
	opts HTTPClientOptions,
) (*sdkmcp.ClientSession, error) {
	if client == nil {
		return nil, errors.New("mcp.DialStreamableHTTP: client must not be nil")
	}
	if endpoint == "" {
		return nil, errors.New("mcp.DialStreamableHTTP: endpoint must not be empty")
	}

	transport := &sdkmcp.StreamableClientTransport{
		Endpoint:             endpoint,
		HTTPClient:           opts.HTTPClient,
		MaxRetries:           opts.MaxRetries,
		DisableStandaloneSSE: opts.DisableStandaloneSSE,
	}

	return client.Connect(ctx, transport, nil)
}

// CommandClientOptions configures a stdio-subprocess MCP client
// connection built by [DialCommand].
type CommandClientOptions struct {
	// Env overrides the subprocess environment. nil inherits the
	// parent process env; a non-nil value (even empty) replaces it.
	// Use [os.Environ] + extra entries when you want to extend rather
	// than replace.
	Env []string

	// Dir sets the subprocess working directory. Empty inherits.
	Dir string

	// TerminateDuration controls how long Close waits after closing
	// stdin for the process to exit before sending SIGTERM. Zero or
	// negative uses the SDK default (5s).
	TerminateDuration time.Duration
}

// DialCommand spawns command as a subprocess and connects to it
// over its stdin/stdout pipes using the MCP stdio transport (the
// canonical local-tool transport — most ecosystem MCP servers
// distribute as `npx -y @modelcontextprotocol/server-<name>` style
// commands).
//
// Caller is responsible for closing the returned session; closing
// it shuts the subprocess down cleanly (stdin EOF, then SIGTERM /
// SIGKILL after TerminateDuration).
//
// Example — list tools from the official filesystem server:
//
//	cli := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "agent", Version: "v0.1"}, nil)
//	session, err := mcp.DialCommand(ctx, cli, "npx",
//	    []string{"-y", "@modelcontextprotocol/server-filesystem", "/workspace"},
//	    mcp.CommandClientOptions{})
//	if err != nil { return err }
//	defer session.Close()
//
// Returns an error when client is nil or command is empty.
func DialCommand(
	ctx context.Context,
	client *sdkmcp.Client,
	command string,
	args []string,
	opts CommandClientOptions,
) (*sdkmcp.ClientSession, error) {
	if client == nil {
		return nil, errors.New("mcp.DialCommand: client must not be nil")
	}
	if command == "" {
		return nil, errors.New("mcp.DialCommand: command must not be empty")
	}

	cmd := exec.CommandContext(ctx, command, args...)
	if opts.Env != nil {
		cmd.Env = opts.Env
	}
	if opts.Dir != "" {
		cmd.Dir = opts.Dir
	}

	transport := &sdkmcp.CommandTransport{Command: cmd}
	if opts.TerminateDuration > 0 {
		transport.TerminateDuration = opts.TerminateDuration
	}

	return client.Connect(ctx, transport, nil)
}

// headerHTTPClient returns an *http.Client that adds static request headers to
// every request — how [ServerConfig.Headers] / [ServerConfig.Authorization]
// reach an access-controlled Streamable HTTP server. It wraps
// http.DefaultTransport so TLS / proxy / redirect behavior is unchanged.
// Returns nil when there is nothing to add (caller then uses the default client).
func headerHTTPClient(authorization string, headers map[string]string) *http.Client {
	if authorization == "" && len(headers) == 0 {
		return nil
	}
	return &http.Client{Transport: &headerRoundTripper{authorization: authorization, headers: headers, base: http.DefaultTransport}}
}

// headerRoundTripper sets static headers on each request, cloning it first so
// the caller's request is never mutated (the [http.RoundTripper] contract). The
// dedicated authorization wins over any "Authorization" in headers, so the
// bearer field is always authoritative.
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

// Transport is the wire mode of an MCP server connection. The zero value
// is invalid — callers pick one explicitly so a misconfigured entry
// fails [ServerConfig.Validate] instead of silently defaulting.
type Transport int

const (
	// TransportHTTP — Streamable HTTP. [ServerConfig.Endpoint] is the URL.
	TransportHTTP Transport = iota + 1
	// TransportStdio — local subprocess over stdin/stdout, the canonical
	// local-tool transport (most ecosystem servers ship as
	// `npx -y @modelcontextprotocol/server-<name>` commands).
	TransportStdio
)

// ServerConfig declaratively describes one MCP server to connect to —
// the analog of an entry in the ubiquitous `mcpServers` config map
// (MCP client applications). [Dial] turns it into a live
// session, dispatching on Transport. Exactly one transport's fields
// apply; the others must be blank. For per-connection tuning (custom
// http.Client, retries, terminate timeout) reach for the lower-level
// [DialStreamableHTTP] / [DialCommand] directly — ServerConfig covers
// the common case.
type ServerConfig struct {
	// Name identifies the server — carried into a [Source] for tool
	// namespacing and surfaced in error messages. Required.
	Name string

	// Transport picks the connection mode. Required (the zero value is
	// rejected by Validate).
	Transport Transport

	// Endpoint is the Streamable HTTP URL. Used when Transport == [TransportHTTP].
	Endpoint string

	// Command is the executable to spawn. Used when Transport == [TransportStdio].
	Command string

	// Args are the command arguments. Used when Transport == [TransportStdio].
	Args []string

	// Env, when non-nil, REPLACES the subprocess environment (it does not
	// extend the parent env — append os.Environ() yourself to extend).
	// Used when Transport == [TransportStdio].
	Env []string

	// Dir sets the subprocess working directory; empty inherits the
	// parent's. Used when Transport == [TransportStdio].
	Dir string

	// Authorization, when set, is sent as the HTTP `Authorization` header on
	// every request to a [TransportHTTP] server — typically "Bearer <token>".
	// It authenticates the client to an access-controlled MCP server. HTTP
	// transport only ([Validate] rejects it for stdio, where a subprocess
	// authenticates through Env, not an HTTP header). When both this and a
	// [Headers] "Authorization" entry are present, this wins.
	Authorization string

	// Headers carries extra static HTTP request headers sent on every request
	// to a [TransportHTTP] server — e.g. an "X-API-Key" some servers require
	// instead of (or alongside) a bearer token. HTTP transport only ([Validate]
	// rejects it for stdio, which has no request headers).
	Headers map[string]string

	// Timeout bounds the connection handshake (the MCP initialize round-trip),
	// applied to both transports. Zero leaves the handshake bounded only by the
	// caller's ctx. It does NOT bound the live session — the session outlives
	// the handshake ctx, so a short timeout can't kill an established
	// connection.
	Timeout time.Duration
}

// Validate reports whether exactly one transport is fully specified and
// the other transport's fields are blank.
func (c ServerConfig) Validate() error {
	if c.Name == "" {
		return errors.New("mcp.ServerConfig: Name is required")
	}
	switch c.Transport {
	case TransportHTTP:
		if c.Endpoint == "" {
			return fmt.Errorf("mcp.ServerConfig %q: Endpoint is required for HTTP transport", c.Name)
		}
		if c.Command != "" {
			return fmt.Errorf("mcp.ServerConfig %q: Command must be empty for HTTP transport", c.Name)
		}
	case TransportStdio:
		if c.Command == "" {
			return fmt.Errorf("mcp.ServerConfig %q: Command is required for stdio transport", c.Name)
		}
		if c.Endpoint != "" {
			return fmt.Errorf("mcp.ServerConfig %q: Endpoint must be empty for stdio transport", c.Name)
		}
		if c.Authorization != "" {
			return fmt.Errorf("mcp.ServerConfig %q: Authorization applies to HTTP transport only (a stdio subprocess authenticates via Env)", c.Name)
		}
		if len(c.Headers) > 0 {
			return fmt.Errorf("mcp.ServerConfig %q: Headers apply to HTTP transport only (a stdio subprocess has no request headers)", c.Name)
		}
	default:
		return fmt.Errorf("mcp.ServerConfig %q: unknown transport %d (set TransportHTTP or TransportStdio)", c.Name, c.Transport)
	}
	return nil
}

// Dial connects client to the MCP server described by cfg and returns
// the initialized session (the caller closes it). It validates cfg, then
// dispatches on Transport to [DialStreamableHTTP] / [DialCommand].
//
// The caller supplies client so it keeps control of client-side options
// (sampling, tools/list_changed handlers, ...) — Dial deliberately does
// not build one. Connect several servers by calling Dial once per
// [ServerConfig] with the same client, wrapping each session in a
// [Source].
func Dial(ctx context.Context, client *sdkmcp.Client, cfg ServerConfig) (*sdkmcp.ClientSession, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	// Bound the initialize handshake — not the live session, which the SDK runs
	// independently of this ctx (so the defer cancel can't sever an established
	// connection).
	if cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
	}
	switch cfg.Transport {
	case TransportHTTP:
		return DialStreamableHTTP(ctx, client, cfg.Endpoint, HTTPClientOptions{
			HTTPClient: headerHTTPClient(cfg.Authorization, cfg.Headers),
		})
	case TransportStdio:
		return DialCommand(ctx, client, cfg.Command, cfg.Args, CommandClientOptions{Env: cfg.Env, Dir: cfg.Dir})
	default:
		// Unreachable after Validate; keeps the switch total.
		return nil, fmt.Errorf("mcp.Dial: unknown transport %d", cfg.Transport)
	}
}
