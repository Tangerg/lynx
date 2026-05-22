// Package http hosts Lyra's HTTP+SSE transport adapter — the
// first concrete realisation of the "transport-agnostic Service
// interface" promise from ARCHITECTURE.md. Web / desktop clients
// POST to /v1/agent/run and consume the AG-UI event stream as SSE.
// CRUD endpoints under /v1/sessions cover session lifecycle.
//
// This adapter is a thin marshal/route layer: every handler
// dispatches to the same Service interfaces the in-process CLI
// uses (chat.Service / session.Service). Adding new transports
// (gRPC, IPC stdio) means a sibling package; the services don't
// change.
package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Tangerg/lynx/lyra/internal/service/approval"
	"github.com/Tangerg/lynx/lyra/internal/service/chat"
	"github.com/Tangerg/lynx/lyra/internal/service/session"
)

// Server is the HTTP+SSE transport. Wire it with the live
// services + listen address, then call [Server.Start] to bind /
// serve and [Server.Shutdown] to drain cleanly.
type Server struct {
	chat     chat.Service
	session  session.Service
	approval approval.Service
	addr     string

	server *http.Server
}

// Config bundles the construction-time inputs. Chat, Session,
// and Addr are required. Approval is optional — when nil the
// /v1/approvals routes return 503; pass a service to enable
// permission gating from the transport surface.
type Config struct {
	Chat     chat.Service
	Session  session.Service
	Approval approval.Service
	Addr     string // e.g. ":8080"
}

// NewServer assembles a Server. Routes are wired here; the
// underlying *http.Server is created lazily in Start so tests can
// inject a custom listener via [Server.Handler].
func NewServer(cfg Config) (*Server, error) {
	if cfg.Chat == nil {
		return nil, errors.New("http: Chat service is required")
	}
	if cfg.Session == nil {
		return nil, errors.New("http: Session service is required")
	}
	if cfg.Addr == "" {
		return nil, errors.New("http: Addr is required")
	}
	return &Server{
		chat:     cfg.Chat,
		session:  cfg.Session,
		approval: cfg.Approval,
		addr:     cfg.Addr,
	}, nil
}

// Handler returns the routed handler — exposed so tests can drive
// it with httptest.NewServer without going through Start. Each
// call builds a fresh mux so concurrent tests don't share state.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("POST /v1/agent/run", s.handleAgentRun)
	mux.HandleFunc("POST /v1/turns/{id}/steer", s.handleSteer)
	mux.HandleFunc("GET /v1/sessions", s.handleListSessions)
	mux.HandleFunc("POST /v1/sessions", s.handleCreateSession)
	mux.HandleFunc("GET /v1/sessions/{id}", s.handleGetSession)
	mux.HandleFunc("DELETE /v1/sessions/{id}", s.handleDeleteSession)
	mux.HandleFunc("GET /v1/approvals", s.handleListApprovals)
	mux.HandleFunc("POST /v1/approvals/{id}", s.handleDecideApproval)
	mux.HandleFunc("GET /v1/approvals/mode", s.handleGetApprovalMode)
	mux.HandleFunc("POST /v1/approvals/mode", s.handleSetApprovalMode)
	return mux
}

// Start binds the listen address and serves until Shutdown is
// called. Returns the listener error verbatim — callers
// distinguish [http.ErrServerClosed] (clean shutdown) from real
// errors.
func (s *Server) Start() error {
	s.server = &http.Server{
		Addr:              s.addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		// No WriteTimeout — SSE streams can be arbitrarily long.
	}
	return s.server.ListenAndServe()
}

// Shutdown gracefully closes the server. Idempotent — calling
// before Start is a no-op.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}

// handleHealth is the liveness probe — returns 200 OK with no
// body. Useful for k8s readiness checks.
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// writeJSONError responds with a JSON-encoded error body and the
// supplied status code. Centralises the response shape so every
// handler reports failures identically.
func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	fmt.Fprintf(w, `{"error":%q}`, msg)
}
