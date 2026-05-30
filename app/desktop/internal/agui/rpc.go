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

	var (
		result any
		rerr   *rpcError
	)
	switch method {
	case "runs.start":
		// Streaming method: return the runId now; AG-UI events flow to the
		// connection's SSE stream as notifications (API.md §3). The conn id
		// rides the Lyra-Connection-Id header so the hub knows which /v1/rpc
		// /stream to push to.
		conn := r.Header.Get("Lyra-Connection-Id")
		if conn == "" {
			rerr = &rpcError{Code: rpcInvalidParams, Message: "runs.start requires the Lyra-Connection-Id header"}
		} else {
			result = map[string]string{"runId": startRunStream(conn, req.Params)}
		}
	case "runs.cancel":
		result, rerr = cancelRunRPC(req.Params)
	default:
		result, rerr = dispatchRPC(method, req.Params)
	}

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

const rpcInvalidParams = -32602

func dispatchRPC(method string, params json.RawMessage) (any, *rpcError) {
	switch method {
	case "runtime.initialize":
		return initializeResult(params)
	case "sessions.list":
		return sessionsPage(), nil
	case "sessions.create":
		return createSession(params)
	case "workspace.projects":
		return projects, nil
	case "workspace.filesChanged":
		return filesChanged, nil
	case "workspace.mcp.list":
		return mcpListLean(), nil
	case "runs.approval.submit":
		return submitApproval(params)
	default:
		return nil, &rpcError{Code: rpcMethodNotFound, Message: "method not found: " + method}
	}
}

// initializeResult answers runtime.initialize (API.md §2/§6.1): echoes the
// negotiated protocol version and advertises what this mock can do. The
// frontend's runtimeStore gates optional UI on capabilities.features.*, and
// won't emit HITL the client didn't declare — so this set keeps the mock's
// demo features (plan / approval / question / mcp / skills) lit.
func initializeResult(params json.RawMessage) (any, *rpcError) {
	var in struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	_ = json.Unmarshal(params, &in)
	pv := in.ProtocolVersion
	if pv == "" {
		pv = "2026-05-28"
	}
	return map[string]any{
		"protocolVersion": pv,
		"serverInfo":      map[string]string{"name": "lyra-agui-mock", "version": "0.1.0"},
		"capabilities": map[string]any{
			"events": map[string]any{
				"standard": []string{
					"RUN_STARTED", "RUN_FINISHED", "RUN_ERROR", "STEP_STARTED", "STEP_FINISHED",
					"TEXT_MESSAGE_START", "TEXT_MESSAGE_CONTENT", "TEXT_MESSAGE_END",
					"TOOL_CALL_START", "TOOL_CALL_ARGS", "TOOL_CALL_END", "TOOL_CALL_RESULT",
					"REASONING_MESSAGE_START", "REASONING_MESSAGE_CONTENT", "REASONING_MESSAGE_END",
					"STATE_SNAPSHOT", "STATE_DELTA", "MESSAGES_SNAPSHOT", "CUSTOM",
				},
				"custom": []string{
					"lyra.plan", "lyra.plan-block", "lyra.code-proposal", "lyra.search-results",
					"lyra.approval", "lyra.approval-result", "lyra.question", "lyra.question-result",
					"lyra.telemetry",
				},
			},
			"features": map[string]any{
				"multimodal": true, "reasoning": true, "checkpoints": false,
				"interrupts": true, "background": false, "subagents": false,
				"skills": true, "mcp": true, "sessionExport": true, "memory": false,
				"attachments": map[string]any{"enabled": true, "maxSizeBytes": 10 * 1024 * 1024},
			},
			"providers": []string{"openai", "anthropic", "moonshot"},
			"limits":    map[string]any{"maxConcurrentRuns": 4},
		},
	}, nil
}

// createSession answers sessions.create — fabricates a fresh idle session
// (CreateSessionRequest { title?, model?, metadata? } → Session, §6.2).
func createSession(params json.RawMessage) (any, *rpcError) {
	var in struct {
		Title    string         `json:"title"`
		Model    string         `json:"model"`
		Metadata map[string]any `json:"metadata"`
	}
	_ = json.Unmarshal(params, &in)
	title := in.Title
	if title == "" {
		title = "New session"
	}
	model := in.Model
	if model == "" {
		model = "gpt-4o"
	}
	md := in.Metadata
	if md == nil {
		md = map[string]any{}
	}
	now := time.Now().Format(time.RFC3339)
	return rpcSession{
		ID:        newID("sess"),
		Title:     title,
		Status:    "idle",
		Model:     model,
		CreatedAt: now,
		UpdatedAt: now,
		Metadata:  md,
	}, nil
}

// submitApproval is the JSON-RPC counterpart of the old POST /permission.
// Wire decision is the protocol's imperative pair "approve" | "deny"
// (API.md §4.3); we map it onto the internal past-tense Decision the
// script goroutine waits on.
func submitApproval(params json.RawMessage) (any, *rpcError) {
	var in struct {
		RequestID string `json:"requestId"`
		Decision  string `json:"decision"`
	}
	if err := json.Unmarshal(params, &in); err != nil || in.RequestID == "" {
		return nil, &rpcError{Code: rpcInvalidParams, Message: "expected { requestId, decision }"}
	}
	var d Decision
	switch in.Decision {
	case "approve":
		d = DecisionApproved
	case "deny":
		d = DecisionDeclined
	default:
		return nil, &rpcError{Code: rpcInvalidParams, Message: `decision must be "approve" | "deny"`}
	}
	if !permissions.resolve(PermissionResponse{RequestID: in.RequestID, Decision: d}) {
		return nil, &rpcError{Code: -32011, Message: "unknown or already-resolved requestId"}
	}
	return struct{}{}, nil
}

// mcpListLean projects the fixture down to the protocol's MCPServer shape
// (API.md §6.5): no `id`, no `icon` — both are client-side presentation
// (the frontend maps `name` → icon itself).
type rpcMCPServer struct {
	Name      string `json:"name"`
	Desc      string `json:"desc"`
	ToolCount int    `json:"toolCount"` // API.md §6.5: count only; details via workspace.mcp.tools
	Status    string `json:"status"`
}

func mcpListLean() []rpcMCPServer {
	out := make([]rpcMCPServer, len(mcpServers))
	for i, s := range mcpServers {
		out[i] = rpcMCPServer{Name: s.Name, Desc: s.Desc, ToolCount: s.Tools, Status: s.Status}
	}
	return out
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

// cancelRunRPC stops an in-flight run started by runs.start.
func cancelRunRPC(params json.RawMessage) (any, *rpcError) {
	var in struct {
		RunID string `json:"runId"`
	}
	if err := json.Unmarshal(params, &in); err != nil || in.RunID == "" {
		return nil, &rpcError{Code: rpcInvalidParams, Message: "expected { runId }"}
	}
	cancelRun(in.RunID) // idempotent — unknown / already-finished runId is a no-op
	return struct{}{}, nil
}

// handleRPCStream is the GET /v1/rpc/stream SSE endpoint. It attaches the
// connection (identified by ?conn=) to the hub and writes every queued
// notification as an SSE `data:` frame, with periodic keepalive comments.
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

	conn := r.URL.Query().Get("conn")
	ch := hub.subscribe(conn)
	defer hub.unsubscribe(conn, ch)

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
		case frame := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", frame)
			flusher.Flush()
		}
	}
}
