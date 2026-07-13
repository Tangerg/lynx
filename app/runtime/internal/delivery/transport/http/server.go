// Package http implements the Lyra Runtime Protocol's streamable-HTTP
// transport. One endpoint carries JSON-RPC:
//
//	POST /v2/rpc/{method}   Request / Notification. A streaming method
//	                        (runs.start/resume/subscribe)
//	                        replies text/event-stream — the response body
//	                        IS the call's event stream (TRANSPORT §6.4);
//	                        everything else replies application/json.
//
// Two sidecars (flat JSON, no envelope, no auth):
//
//	GET /v2/info              ServerInfo + protocolVersion + capabilities
//	GET /v2/health            liveness probe
//
// See docs/{API,TRANSPORT}.md for the wire details. The middleware here wraps
// each request in an OTel span and sets the X-Method header — the dispatcher
// itself stays transport-agnostic.
package http

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/dispatch"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
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

// Server is the HTTP transport. One instance per process — a thin
// adapter over the dispatcher: it decodes a POST, dispatches, and either
// writes one application/json reply or (for streaming methods) streams
// the call's event channel as text/event-stream (TRANSPORT §6.4). It
// holds no per-run state — the event hubs + replay live in the runtime.
type Server struct {
	info     protocol.DiscoverResponse
	serverID string

	localToken      string
	corsOrigins     []string
	healthProbes    []*healthProbeRunner
	agentDocsLister AgentDocsLister

	dispatcher messageHandler

	httpServer *http.Server

	mu      sync.Mutex
	started bool
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
	// `Authorization: Bearer <LocalToken>` — streaming POSTs included
	// (no header-less EventSource to exempt under streamable HTTP). Only
	// the sidecars bypass. Empty disables the gate — tests + same-origin
	// TUI scenarios.
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
	s := &Server{
		serverID:        serverID,
		localToken:      cfg.LocalToken,
		corsOrigins:     slices.Clone(cfg.CORSOrigins),
		healthProbes:    newHealthProbeRunners(cfg.HealthProbes),
		agentDocsLister: cfg.AgentDocsLister,
		dispatcher:      dispatch.New(cfg.Runtime),
		info: protocol.DiscoverResponse{
			ProtocolVersion: cfg.ProtocolVersion,
			ServerInfo:      cfg.ServerInfo,
			Capabilities:    cloneServerCapabilities(cfg.Capabilities),
		},
	}
	s.httpServer = &http.Server{
		Addr:              cfg.Addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		// No WriteTimeout — SSE streams can be arbitrarily long.
	}
	return s, nil
}

func cloneServerCapabilities(in protocol.ServerCapabilities) protocol.ServerCapabilities {
	in.Events = slices.Clone(in.Events)
	in.StreamingMethods = slices.Clone(in.StreamingMethods)
	in.Features = maps.Clone(in.Features)
	in.Providers = slices.Clone(in.Providers)
	return in
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

	// JSON-RPC body endpoint — the only RPC entry (streamable HTTP,
	// TRANSPORT §6.1): a streaming method's events ride its own POST
	// response (text/event-stream), so there is no separate stream
	// endpoint. Single form: method MUST appear in the URL path (dotted,
	// single segment); bare `/v2/rpc` has no matching route ⇒ chi 404.
	r.Post("/v2/rpc/{method}", s.handleRPCWithMethod)

	return r
}

// Start binds the listen address and serves until Shutdown is called.
// Returns http.ErrServerClosed on clean shutdown.
func (s *Server) Start() error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return errors.New("http: server already started")
	}
	s.started = true
	srv := s.httpServer
	s.mu.Unlock()
	return srv.ListenAndServe()
}

// Shutdown gracefully drains the server. It is safe before Start and prevents
// a later Start from binding. Active handlers keep their request contexts until
// they return or the caller's shutdown deadline expires; run ownership remains
// in the runtime rather than this transport.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// Close force-closes listeners and active connections. The process owner uses
// it only when graceful Shutdown exhausts its deadline, to cancel active
// request contexts before application resources are released.
func (s *Server) Close() error {
	return s.httpServer.Close()
}
