package agui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Minimal JSON-RPC 2.0 surface for the Lyra Runtime Protocol cutover
// (docs/API.md §1, §5.1). The frontend's rpc/ stack POSTs to
// /v1/rpc/{method} and reads server-initiated notifications from the SSE
// stream at /v1/rpc/stream. This mock implements only the methods the
// cutover has reached so far; the REST endpoints in rest.go stay live for
// everything still on the old fetch path.
//
// Wire rules honored: method name is carried verbatim in the URL (dots
// preserved, no slashes); HTTP status reflects transport only; business
// errors ride in the JSON-RPC `error` object.

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

const rpcMethodNotFound = -32601

// handleRPC dispatches POST /v1/rpc/{method}. The single Response (if any)
// comes back in this POST's body — the frontend transport pushes it into
// the same channel the SSE notifications feed, and the client correlates
// by id.
func (s *Server) handleRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	method := strings.TrimPrefix(r.URL.Path, "/v1/rpc/")
	var req rpcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
		return
	}

	result, rerr := dispatchRPC(method, req.Params)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	resp := rpcResponse{JSONRPC: "2.0", ID: req.ID}
	if rerr != nil {
		resp.Error = rerr
	} else {
		resp.Result = result
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func dispatchRPC(method string, _ json.RawMessage) (any, *rpcError) {
	switch method {
	case "sessions.list":
		return sessionsPage(), nil
	default:
		return nil, &rpcError{Code: rpcMethodNotFound, Message: "method not found: " + method}
	}
}

// rpcSession mirrors the frontend rpc/shapes.ts `Session` (richer than the
// lean REST Session): it carries createdAt/updatedAt + a metadata bag the
// protocol shape requires. The frontend maps it back down to its sidebar
// row shape.
type rpcSession struct {
	ID        string         `json:"id"`
	Title     string         `json:"title"`
	Status    string         `json:"status"`
	Model     string         `json:"model"`
	CreatedAt string         `json:"createdAt"`
	UpdatedAt string         `json:"updatedAt"`
	Metadata  map[string]any `json:"metadata"`
}

// page mirrors the frontend `Page<T>` envelope.
type page struct {
	Items   any  `json:"items"`
	HasMore bool `json:"hasMore"`
}

func sessionsPage() page {
	lean := makeSessions()
	items := make([]rpcSession, len(lean))
	for i, s := range lean {
		items[i] = rpcSession{
			ID:        s.ID,
			Title:     s.Title,
			Status:    s.Status,
			Model:     s.Model,
			CreatedAt: s.Time,
			UpdatedAt: s.Time,
			Metadata:  map[string]any{},
		}
	}
	return page{Items: items, HasMore: false}
}

// handleRPCStream is the GET /v1/rpc/stream SSE endpoint. The mock has no
// server-initiated notifications to push yet, so it just holds the
// connection open with periodic keepalive comments — enough for the
// frontend client's recv() pump to attach without retry spam.
func (s *Server) handleRPCStream(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	ctx := r.Context()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}
