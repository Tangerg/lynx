// Package http implements the Lyra Runtime Protocol's streamable-HTTP
// transport. One endpoint carries JSON-RPC:
//
//	POST /v2/rpc            Request / Notification. A streaming method
//	                        (runs.start/resume/subscribe)
//	                        replies text/event-stream — the response body
//	                        IS the call's event stream (TRANSPORT §6.4);
//	                        everything else replies application/json.
//
// Operational sidecars use typed JSON without an envelope or auth:
//
//	GET /v2/info              Public server and protocol identity
//	GET /v2/health/live       Process liveness
//	GET /v2/health/ready      Dependency readiness
//
// See docs/{API,TRANSPORT}.md for the wire details. The middleware here wraps
// each request in an OTel span and sets the X-Method header — the dispatcher
// itself stays transport-agnostic.
package http

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Tangerg/lynx/app/runtime/internal/component/idempotency"
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
	Handle(ctx context.Context, msg transport.Message) dispatch.HandleResult
}

// Server is the HTTP transport. One instance per process — a thin
// adapter over the dispatcher: it decodes a POST, dispatches, and either
// writes one application/json reply or (for streaming methods) streams
// the call's event channel as text/event-stream (TRANSPORT §6.4). It
// holds no per-run state — the event hubs + replay live in the runtime.
type Server struct {
	info     infoResponse
	serverID string

	localToken   string
	corsOrigins  []string
	healthProbes []*healthProbeRunner

	dispatcher messageHandler

	httpServer   *http.Server
	handlerCtx   context.Context
	stopHandlers context.CancelFunc

	mu      sync.Mutex
	started bool
}

// Config bundles construction inputs.
type Config struct {
	// Runtime is the Runtime implementation. Required.
	Runtime protocol.Runtime

	// Addr is the listen address (":8080", "127.0.0.1:0", ...). Required.
	Addr string

	// ServerInfo + ProtocolVersion populate the
	// /v2/info sidecar response. Required.
	ServerInfo      protocol.ServerInfo
	ProtocolVersion string

	// ServerID identifies this process in X-Server response
	// header. Defaults to ServerInfo.Name + "/" + ServerInfo.Version.
	ServerID string

	// LocalToken, when non-empty, gates POST /v2/rpc with
	// `Authorization: Bearer <LocalToken>` — streaming POSTs included
	// (no header-less EventSource to exempt under streamable HTTP). Only
	// the sidecars bypass. Empty disables the gate — tests + same-origin
	// TUI scenarios.
	LocalToken string

	// CORSOrigins is the exact-match origin allowlist; "*" is honored
	// (without credentials). Empty disables CORS — same-origin only.
	CORSOrigins []string

	// HealthProbes are the labeled readiness checks invoked on every
	// GET /v2/health/ready. Empty list ⇒ the endpoint always returns ready.
	// Probes run in parallel under a shared 2s budget.
	HealthProbes []HealthProbe

	// IdempotencyStore persists first responses for Idempotency-Key replay. nil
	// uses the dispatcher's process-local store (appropriate for tests).
	IdempotencyStore idempotency.Store
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
	seenProbes := make(map[string]struct{}, len(cfg.HealthProbes))
	for _, probe := range cfg.HealthProbes {
		if probe.Name == "" {
			return nil, errors.New("http: health probe name is required")
		}
		if probe.Probe == nil {
			return nil, errors.New("http: health probe " + probe.Name + " has no function")
		}
		if _, exists := seenProbes[probe.Name]; exists {
			return nil, errors.New("http: duplicate health probe name: " + probe.Name)
		}
		seenProbes[probe.Name] = struct{}{}
	}
	serverID := cfg.ServerID
	if serverID == "" {
		serverID = cfg.ServerInfo.Name + "/" + cfg.ServerInfo.Version
	}
	handlerCtx, stopHandlers := context.WithCancel(context.Background())
	s := &Server{
		serverID:     serverID,
		localToken:   cfg.LocalToken,
		corsOrigins:  slices.Clone(cfg.CORSOrigins),
		healthProbes: newHealthProbeRunners(cfg.HealthProbes),
		dispatcher:   dispatch.New(cfg.Runtime, dispatch.WithIdempotencyStore(cfg.IdempotencyStore)),
		handlerCtx:   handlerCtx,
		stopHandlers: stopHandlers,
		info:         newInfoResponse(cfg.ServerInfo, cfg.ProtocolVersion),
	}
	s.httpServer = &http.Server{
		Addr:              cfg.Addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		// No WriteTimeout — SSE streams can be arbitrarily long.
	}
	return s, nil
}

// Handler returns the routed handler — exposed so tests can drive it
// with httptest.NewServer without going through Start. Each call
// builds a fresh mux so concurrent tests don't share state.
//
// Middleware order (outer → inner):
//
//	server lifecycle → observability → cors → authGate → mux
//
// The lifecycle wrapper owns transport cancellation. Observability remains
// outside the protocol middleware so every request (including CORS preflight
// and 401) is logged; cors precedes authGate so OPTIONS preflights resolve
// without a token; authGate precedes the mux so unauthenticated requests never
// touch handlers.
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(s.observability, corsMiddleware(s.corsOrigins), s.authGate)

	// Sidecars — flat JSON, must NOT go through the JSON-RPC envelope.
	r.Get(infoPath, s.handleInfo)
	r.Get(livenessPath, s.handleLiveness)
	r.Get(readinessPath, s.handleReadiness)

	// A streaming method's events ride its own POST response, so there is no
	// separate stream endpoint. The envelope is the sole owner of method identity.
	r.Post(rpcPath, s.serveRPC)

	return s.withServerLifecycle(r)
}

// withServerLifecycle cancels transport-owned request work when this server is
// shutting down. Runtime-owned runs are deliberately not tied to this context:
// canceling a streaming request only detaches that client from the run.
func (s *Server) withServerLifecycle(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithCancel(r.Context())
		stop := context.AfterFunc(s.handlerCtx, cancel)
		defer func() {
			stop()
			cancel()
		}()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
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

// Shutdown stops transport-owned request work, then gracefully drains the
// server. It is safe before Start and prevents a later Start from binding.
// Runtime-owned runs continue independently after their clients detach.
func (s *Server) Shutdown(ctx context.Context) error {
	s.stopHandlers()
	return s.httpServer.Shutdown(ctx)
}

// Close force-closes listeners and active connections. The process owner uses
// it only when graceful Shutdown exhausts its deadline, to cancel active
// request contexts before application resources are released.
func (s *Server) Close() error {
	s.stopHandlers()
	return s.httpServer.Close()
}
