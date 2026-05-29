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

	sdkevents "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	sdksse "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/encoding/sse"
)

// DefaultAddr is the loopback-only address the mock listens on.
// Loopback because this is a local desktop app and we don't want to bind
// to any network interface.
const DefaultAddr = "127.0.0.1:17171"

// Server is an AG-UI mock server that streams scripted runs over SSE.
//
// One Server per app. Start it from OnStartup, Stop from OnShutdown.
type Server struct {
	Addr   string
	server *http.Server

	mu      sync.Mutex
	started bool
}

func New(addr string) *Server {
	if addr == "" {
		addr = DefaultAddr
	}
	return &Server{Addr: addr}
}

// Start launches the HTTP listener in a goroutine. Returns once the listener
// is bound, so callers know the URL is reachable before returning.
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/run", s.handleRun)
	// Lyra Runtime Protocol (JSON-RPC). Exact-match /v1/rpc/stream wins
	// over the /v1/rpc/ subtree dispatcher per ServeMux longest-pattern.
	// HITL approval now rides runs.approval.submit (rpc.go) — the old
	// POST /permission endpoint is retired.
	mux.HandleFunc("/v1/rpc/stream", s.handleRPCStream)
	mux.HandleFunc("/v1/rpc/", s.handleRPC)
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	s.registerREST(mux)

	listener, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return fmt.Errorf("agui: bind %s: %w", s.Addr, err)
	}

	s.server = &http.Server{
		Handler:           withCORS(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		if err := s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("agui: server stopped: %v", err)
		}
	}()
	s.started = true
	log.Printf("agui: mock server listening on http://%s", s.Addr)
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

// handleRun is the AG-UI /run endpoint:
//   POST application/json (RunAgentInput) → text/event-stream
//
// Events are produced by the community Go SDK and framed by its SSEWriter,
// which handles newline escaping in JSON payloads and the flush-per-event
// dance for us.
func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if _, ok := w.(http.Flusher); !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	var input RunAgentInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx-style buffering, harmless locally
	w.WriteHeader(http.StatusOK)

	// Cancel the run when the client disconnects (closes the connection or
	// aborts the fetch). AbstractAgent.abortRun() triggers this path.
	ctx := r.Context()

	writer := sdksse.NewSSEWriter()
	emit := func(ev sdkevents.Event) error {
		return writer.WriteEvent(ctx, w, ev)
	}

	Run(ctx, input, emit)
}

// withCORS allows the Wails webview origin and Vite dev origin to call us.
// Loopback-only listener + permissive CORS is the right tradeoff for a desktop
// app — the surface is the user's own machine.
func withCORS(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Last-Event-Id, Lyra-Connection-Id")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}
