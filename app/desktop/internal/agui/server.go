// Package agui hosts the local dev mock for the Lyra Runtime Protocol v2
// (docs/API.md + docs/TRANSPORT.md). It speaks JSON-RPC 2.0 over HTTP:
//
//	POST /v2/rpc/{method}      — request / client notification
//	GET  /v2/rpc/stream?conn=  — SSE stream of server notifications
//	GET  /v2/info | /v2/health — sidecar metadata (pre-handshake)
//	GET  /plugins[/...]        — sideload plugin bundles (see plugins.go)
//
// It streams a scripted Session→Run→Item turn so the desktop app has
// something believable to render without a real runtime. No external
// protocol SDK — the v2 wire shapes are plain structs here.
package agui

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

// DefaultAddr is the loopback-only address the mock listens on.
const DefaultAddr = "127.0.0.1:17171"

// ProtocolVersion is the v2 baseline this mock implements.
const ProtocolVersion = "2026-06-03"

// Server is a v2 Runtime Protocol mock. One per app: Start from OnStartup,
// Stop from OnShutdown.
type Server struct {
	Addr   string
	server *http.Server

	mu      sync.Mutex
	started bool

	hub *hub // SSE connection registry
	rt  *runtime
}

func New(addr string) *Server {
	if addr == "" {
		addr = DefaultAddr
	}
	h := newHub()
	return &Server{Addr: addr, hub: h, rt: newRuntime(h)}
}

func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return nil
	}

	listener, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return fmt.Errorf("agui: bind %s: %w", s.Addr, err)
	}
	s.server = &http.Server{Handler: s.handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("agui: server stopped: %v", err)
		}
	}()
	s.started = true
	log.Printf("agui: v2 mock listening on http://%s", s.Addr)
	return nil
}

func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started || s.server == nil {
		return nil
	}
	s.started = false
	return s.server.Shutdown(ctx)
}

// handler builds the route table (shared by Start + tests). Exact-match
// /v2/rpc/stream wins over the /v2/rpc/ subtree per ServeMux longest-pattern.
func (s *Server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/rpc/stream", s.handleStream)
	mux.HandleFunc("/v2/rpc/", s.handleRPC)
	mux.HandleFunc("/v2/info", s.handleInfo)
	mux.HandleFunc("/v2/health", s.handleHealth)
	mux.HandleFunc("/plugins", s.handlePluginsList)
	mux.HandleFunc("/plugins/", s.handlePluginAsset)
	return withCORS(mux)
}

// ---------------------------------------------------------------------------
// HTTP handlers
// ---------------------------------------------------------------------------

// handleRPC dispatches POST /v2/rpc/{method}. A request gets a JSON-RPC
// response (200); a notification gets a bare 202 (TRANSPORT.md §6.3).
func (s *Server) handleRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req rpcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
		return
	}
	connID := r.Header.Get("X-Conn-Id")

	// Notifications (no id) — ack with 202, no body.
	if req.ID == "" {
		s.rt.dispatchNotification(req.Method, req.Params)
		w.WriteHeader(http.StatusAccepted)
		return
	}

	result, rpcErr := s.rt.dispatch(connID, req.Method, req.Params)
	resp := rpcResponse{JSONRPC: "2.0", ID: req.ID}
	if rpcErr != nil {
		resp.Error = rpcErr
	} else {
		resp.Result = result
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Method", req.Method)
	w.Header().Set("X-Server", "lyra-mock")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// handleStream is the SSE notification channel for one connection.
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	connID := r.URL.Query().Get("conn")
	if connID == "" {
		connID = r.Header.Get("X-Conn-Id")
	}
	if connID == "" {
		http.Error(w, "missing conn", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	ch := s.hub.connect(connID)
	defer s.hub.disconnect(connID)

	ctx := r.Context()
	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ping.C:
			_, _ = fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case n, more := <-ch:
			if !more {
				return
			}
			// One JSON-RPC notification per SSE frame; SSE id == eventId.
			_, _ = fmt.Fprintf(w, "id: %s\nevent: message\ndata: %s\n\n", n.eventID, n.data)
			flusher.Flush()
		}
	}
}

func (s *Server) handleInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, infoResponse())
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// withCORS allows the Wails webview + Vite dev origins. Loopback-only listener
// + permissive CORS is the right tradeoff for a local desktop app.
func withCORS(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers",
			"Content-Type, Authorization, Last-Event-Id, X-Conn-Id, X-Trace-Id, X-Protocol-Version, X-Idempotency-Key")
		w.Header().Set("Access-Control-Expose-Headers", "X-Method, X-Server, X-Trace-Id")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// ---------------------------------------------------------------------------
// SSE connection hub
// ---------------------------------------------------------------------------

type notification struct {
	eventID string
	data    string // pre-encoded JSON-RPC notification frame
}

type hub struct {
	mu    sync.Mutex
	conns map[string]chan notification
}

func newHub() *hub { return &hub{conns: map[string]chan notification{}} }

func (h *hub) connect(connID string) chan notification {
	h.mu.Lock()
	defer h.mu.Unlock()
	// Replace any stale channel for this conn id (reconnect).
	if old, ok := h.conns[connID]; ok {
		close(old)
	}
	ch := make(chan notification, 256)
	h.conns[connID] = ch
	return ch
}

func (h *hub) disconnect(connID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if ch, ok := h.conns[connID]; ok {
		delete(h.conns, connID)
		close(ch)
	}
}

// send pushes a notification frame to one connection. Best-effort: drops if
// the connection is gone or its buffer is full (ephemeral deltas are
// recoverable per the durable/ephemeral invariant, API.md §5.2).
func (h *hub) send(connID string, n notification) {
	h.mu.Lock()
	ch, ok := h.conns[connID]
	h.mu.Unlock()
	if !ok {
		return
	}
	select {
	case ch <- n:
	default:
	}
}
