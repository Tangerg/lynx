package mcp

import (
	"context"
	"errors"
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
// 2025-03-26 (replaces the legacy two-endpoint SSE transport that
// spring-ai has deprecated in c549bf821).
//
// Mount the returned handler on any standard library / framework
// router:
//
//	mux := http.NewServeMux()
//	mux.Handle("/mcp", mcp.NewStreamableHTTPHandler(server, nil))
//	_ = http.ListenAndServe(":8080", mux)
//
// Multi-tenant routing example:
//
//	h := mcp.NewStreamableHTTPHandler(nil, &mcp.HTTPServerOptions{
//	    GetServer: func(r *http.Request) *sdkmcp.Server {
//	        return registry[r.PathValue("tenant")]
//	    },
//	})
func NewStreamableHTTPHandler(server *sdkmcp.Server, opts *HTTPServerOptions) http.Handler {
	o := HTTPServerOptions{}
	if opts != nil {
		o = *opts
	}

	getServer := o.GetServer
	if getServer == nil {
		// Capture the server pointer so the closure is independent of
		// the option struct after this constructor returns.
		captured := server
		getServer = func(*http.Request) *sdkmcp.Server { return captured }
	}

	return sdkmcp.NewStreamableHTTPHandler(getServer, &sdkmcp.StreamableHTTPOptions{
		Stateless:    o.Stateless,
		JSONResponse: o.JSONResponse,
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
//	    "https://mcp.example.com/", nil)
//	if err != nil { return err }
//	defer session.Close()
//
//	res, err := session.ListTools(ctx, nil)
//	...
//
// Returns an error when client or endpoint is empty.
func DialStreamableHTTP(
	ctx context.Context,
	client *sdkmcp.Client,
	endpoint string,
	opts *HTTPClientOptions,
) (*sdkmcp.ClientSession, error) {
	if client == nil {
		return nil, errors.New("mcp.DialStreamableHTTP: client must not be nil")
	}
	if endpoint == "" {
		return nil, errors.New("mcp.DialStreamableHTTP: endpoint must not be empty")
	}

	transport := &sdkmcp.StreamableClientTransport{
		Endpoint: endpoint,
	}
	if opts != nil {
		transport.HTTPClient = opts.HTTPClient
		transport.MaxRetries = opts.MaxRetries
		transport.DisableStandaloneSSE = opts.DisableStandaloneSSE
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
//	    nil)
//	if err != nil { return err }
//	defer session.Close()
//
// Returns an error when client or command is empty.
func DialCommand(
	ctx context.Context,
	client *sdkmcp.Client,
	command string,
	args []string,
	opts *CommandClientOptions,
) (*sdkmcp.ClientSession, error) {
	if client == nil {
		return nil, errors.New("mcp.DialCommand: client must not be nil")
	}
	if command == "" {
		return nil, errors.New("mcp.DialCommand: command must not be empty")
	}

	cmd := exec.CommandContext(ctx, command, args...)
	if opts != nil {
		if opts.Env != nil {
			cmd.Env = opts.Env
		}
		if opts.Dir != "" {
			cmd.Dir = opts.Dir
		}
	}

	transport := &sdkmcp.CommandTransport{Command: cmd}
	if opts != nil && opts.TerminateDuration > 0 {
		transport.TerminateDuration = opts.TerminateDuration
	}

	return client.Connect(ctx, transport, nil)
}
