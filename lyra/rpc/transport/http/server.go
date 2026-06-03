// Package http implements the Lyra Runtime Protocol's HTTP
// transport. Two endpoints carry JSON-RPC:
//
//	POST /v2/rpc[/{method}]   Request/Notification (recommended path
//	                          includes method for ops-friendliness)
//	GET  /v2/rpc/stream       SSE — server → client notifications
//
// Two sidecars (flat JSON, no envelope, no auth):
//
//	GET /v2/info              ServerInfo + protocolVersion + capabilities
//	GET /v2/health            liveness probe
//
// See docs/{API,TRANSPORT}.md for the wire details. Most of the
// observability discipline (X-Method header, structured log,
// metric labels) is enforced here in middleware — the dispatcher
// itself stays transport-agnostic.
package http

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Tangerg/lynx/lyra/rpc/dispatch"
	"github.com/Tangerg/lynx/lyra/rpc/protocol"
	"github.com/Tangerg/lynx/lyra/rpc/transport"
)

// messageHandler is the dispatch surface this transport needs: route
// one inbound message, return the synchronous reply plus any stream.
// Defined here (consumer side) so the transport depends on the single
// method it calls rather than the concrete *dispatch.Dispatcher — the
// dispatcher's per-conn state stays its own concern, and tests can
// inject a fake without standing up a Runtime.
type messageHandler interface {
	Handle(ctx context.Context, msg transport.Message, expectedMethod string) dispatch.HandleResult
}

// Server is the HTTP transport. One instance per process — it owns
// the dispatcher, the per-stream replay buffers, and the inbound
// connection registry for the SSE channel.
type Server struct {
	api      protocol.Runtime
	addr     string
	info     protocol.InitializeResponse
	serverID string

	localToken      string
	corsOrigins     []string
	healthProbes    []HealthProbe
	agentDocsLister AgentDocsLister

	dispatcher messageHandler
	streams    *streamRegistry
	clients    *clientRegistry

	// baseCtx bounds detached background work (the SSE event-pump
	// goroutines started by attachStream) to the server's lifetime: it
	// outlives any single request — so a pump keeps broadcasting after
	// the triggering request returns — yet is canceled by Shutdown so
	// pumps don't leak. A server owns its own lifetime, so Background is
	// the correct root here; request handlers must NOT hand their
	// per-request ctx to a pump (it would cancel the stream the moment
	// the response is written).
	baseCtx    context.Context
	baseCancel context.CancelFunc

	httpServer *http.Server

	mu sync.Mutex
}

// Config bundles construction inputs.
type Config struct {
	// Runtime is the Runtime implementation. Required.
	Runtime protocol.Runtime

	// Addr is the listen address (":8080", "127.0.0.1:0", ...). Required.
	Addr string

	// ServerInfo + ProtocolVersion + Capabilities populate the
	// /v2/info sidecar response. Required.
	ServerInfo      protocol.ServerInfo
	ProtocolVersion string
	Capabilities    protocol.ServerCapabilities

	// ServerID identifies this process in X-Server response
	// header. Defaults to ServerInfo.Name + "/" + ServerInfo.Version.
	ServerID string

	// LocalToken, when non-empty, gates every POST /v2/rpc/* with
	// `Authorization: Bearer <LocalToken>`. Sidecars and the SSE
	// stream bypass (TRANSPORT.md §4.3 mirrors FE's withCredentials:
	// false EventSource). Empty disables the gate — tests + same-
	// origin TUI scenarios.
	LocalToken string

	// CORSOrigins is the exact-match origin allowlist; "*" is honored
	// (without credentials). Empty disables CORS — same-origin only.
	CORSOrigins []string

	// HealthProbes are the labeled liveness checks invoked on every
	// GET /v2/health. Empty list ⇒ the endpoint always returns ok.
	// Probes run in parallel under a shared 2s budget.
	HealthProbes []HealthProbe

	// AgentDocsLister, when non-nil, is called on every GET /v2/info
	// to surface the AGENTS.md files the engine would inject into
	// the system prompt for the server's working directory. Listed
	// under the `agentDocs` field (path + size only — content stays
	// out of the response to keep oncall payloads small). Nil
	// omits the field entirely.
	AgentDocsLister AgentDocsLister
}

// AgentDocsLister returns the AGENTS.md files currently visible to
// the runtime. Paths are absolute; Bytes is the on-disk size of
// the file's trimmed content. Implementations should be cheap —
// the function fires on every /v2/info hit.
type AgentDocsLister func(ctx context.Context) []AgentDocInfo

// AgentDocInfo is one entry in the /v2/info.agentDocs array.
type AgentDocInfo struct {
	Path  string `json:"path"`
	Bytes int    `json:"bytes"`
}

// NewServer assembles a Server.
func NewServer(cfg Config) (*Server, error) {
	if cfg.Runtime == nil {
		return nil, errors.New("http: Runtime is required")
	}
	if cfg.Addr == "" {
		return nil, errors.New("http: Addr is required")
	}
	if cfg.ProtocolVersion == "" {
		return nil, errors.New("http: ProtocolVersion is required")
	}
	serverID := cfg.ServerID
	if serverID == "" {
		serverID = cfg.ServerInfo.Name + "/" + cfg.ServerInfo.Version
	}
	baseCtx, baseCancel := context.WithCancel(context.Background())
	return &Server{
		api:             cfg.Runtime,
		addr:            cfg.Addr,
		serverID:        serverID,
		localToken:      cfg.LocalToken,
		corsOrigins:     cfg.CORSOrigins,
		healthProbes:    cfg.HealthProbes,
		agentDocsLister: cfg.AgentDocsLister,
		dispatcher:      dispatch.New(cfg.Runtime),
		streams:         newStreamRegistry(),
		clients:         newClientRegistry(),
		baseCtx:         baseCtx,
		baseCancel:      baseCancel,
		info: protocol.InitializeResponse{
			ProtocolVersion: cfg.ProtocolVersion,
			ServerInfo:      cfg.ServerInfo,
			Capabilities:    cfg.Capabilities,
		},
	}, nil
}

// Handler returns the routed handler — exposed so tests can drive it
// with httptest.NewServer without going through Start. Each call
// builds a fresh mux so concurrent tests don't share state.
//
// Middleware order (outer → inner):
//
//	observability → cors → authGate → mux
//
// observability outermost so every request (including CORS preflight
// and 401) is logged; cors before authGate so OPTIONS preflights
// resolve without a token; authGate before mux so unauth requests
// never touch handlers.
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(s.observability, corsMiddleware(s.corsOrigins), s.authGate)

	// Sidecars — flat JSON, must NOT go through the JSON-RPC envelope.
	r.Get("/v2/info", s.handleInfo)
	r.Get("/v2/health", s.handleHealth)

	// Streaming notifications (SSE).
	r.Get("/v2/rpc/stream", s.handleStream)

	// JSON-RPC body endpoint. Single form per API.md §10.1: method MUST
	// appear in the URL path (dotted, single segment). Bare `/v2/rpc` has
	// no matching route ⇒ chi 404 — greenfield, no fallback registered.
	r.Post("/v2/rpc/{method}", s.handleRPCWithMethod)

	return r
}

// Start binds the listen address and serves until Shutdown is called.
// Returns http.ErrServerClosed on clean shutdown.
func (s *Server) Start() error {
	s.mu.Lock()
	s.httpServer = &http.Server{
		Addr:              s.addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		// No WriteTimeout — SSE streams can be arbitrarily long.
	}
	srv := s.httpServer
	s.mu.Unlock()
	return srv.ListenAndServe()
}

// Shutdown gracefully closes the server. Idempotent. Cancels baseCtx so
// every detached SSE event pump stops, then closes inbound clients and
// the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.baseCancel()
	s.mu.Lock()
	srv := s.httpServer
	s.mu.Unlock()
	if srv == nil {
		return nil
	}
	s.clients.closeAll()
	return srv.Shutdown(ctx)
}
