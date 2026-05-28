// Package http implements the Lyra Runtime Protocol's HTTP
// transport. Two endpoints carry JSON-RPC:
//
//	POST /v1/rpc[/{method}]   Request/Notification (recommended path
//	                          includes method for ops-friendliness)
//	GET  /v1/rpc/stream       SSE — server → client notifications
//
// Two sidecars (flat JSON, no envelope, no auth):
//
//	GET /v1/info              ServerInfo + protocolVersion + capabilities
//	GET /v1/health            liveness probe
//
// See docs/{API,TRANSPORT}.md for the wire details. Most of the
// observability discipline (X-Lyra-Method header, structured log,
// metric labels) is enforced here in middleware — the dispatcher
// itself stays transport-agnostic.
package http

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/Tangerg/lynx/lyra/pkg/coreapi"
	"github.com/Tangerg/lynx/lyra/pkg/rpcadapter"
)

// Server is the HTTP transport. One instance per process — it owns
// the dispatcher, the per-stream replay buffers, and the inbound
// connection registry for the SSE channel.
type Server struct {
	api      coreapi.CoreAPI
	addr     string
	info     coreapi.InitializeOut
	serverID string

	dispatcher *rpcadapter.Dispatcher
	streams    *streamRegistry
	clients    *clientRegistry

	httpServer *http.Server

	mu sync.Mutex
}

// Config bundles construction inputs.
type Config struct {
	// API is the CoreAPI implementation. Required.
	API coreapi.CoreAPI

	// Addr is the listen address (":8080", "127.0.0.1:0", ...). Required.
	Addr string

	// ServerInfo + ProtocolVersion + Capabilities populate the
	// /v1/info sidecar response. Required.
	ServerInfo      coreapi.ServerInfo
	ProtocolVersion string
	Capabilities    coreapi.ServerCapabilities

	// ServerID identifies this process in X-Lyra-Server response
	// header. Defaults to ServerInfo.Name + "/" + ServerInfo.Version.
	ServerID string
}

// NewServer assembles a Server.
func NewServer(cfg Config) (*Server, error) {
	if cfg.API == nil {
		return nil, errors.New("http: API is required")
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
	return &Server{
		api:        cfg.API,
		addr:       cfg.Addr,
		serverID:   serverID,
		dispatcher: rpcadapter.New(cfg.API),
		streams:    newStreamRegistry(),
		clients:    newClientRegistry(),
		info: coreapi.InitializeOut{
			ProtocolVersion: cfg.ProtocolVersion,
			ServerInfo:      cfg.ServerInfo,
			Capabilities:    cfg.Capabilities,
		},
	}, nil
}

// Handler returns the routed handler — exposed so tests can drive it
// with httptest.NewServer without going through Start. Each call
// builds a fresh mux so concurrent tests don't share state.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Sidecar — must NOT go through JSON-RPC envelope.
	mux.HandleFunc("GET /v1/info", s.handleInfo)
	mux.HandleFunc("GET /v1/health", s.handleHealth)

	// JSON-RPC body endpoint. Single form per API.md v4 §10.1: method
	// MUST appear in the URL path. Requests to `/v1/rpc` (no method
	// suffix) fall through the mux to 404 — greenfield, no fallback
	// route registered.
	mux.HandleFunc("POST /v1/rpc/{method...}", s.handleRPCWithMethod)

	// Streaming notifications (SSE).
	mux.HandleFunc("GET /v1/rpc/stream", s.handleStream)

	return s.observability(mux)
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

// Shutdown gracefully closes the server. Idempotent.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	srv := s.httpServer
	s.mu.Unlock()
	if srv == nil {
		return nil
	}
	s.clients.closeAll()
	return srv.Shutdown(ctx)
}

// Addr returns the actual listen address (useful in tests where
// Addr was given as ":0"). Empty if Start has not been called.
func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.httpServer == nil {
		return s.addr
	}
	return s.httpServer.Addr
}
