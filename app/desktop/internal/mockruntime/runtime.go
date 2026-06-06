package mockruntime

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// JSON-RPC envelope (API.md §1)
// ---------------------------------------------------------------------------

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      string          `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcError struct {
	Code    int            `json:"code"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data,omitempty"`
}

type rpcResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      string    `json:"id"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

func errMethodNotFound(method string) *rpcError {
	return &rpcError{Code: -32601, Message: "method not found: " + method,
		Data: map[string]any{"type": "method_not_found"}}
}

// ---------------------------------------------------------------------------
// runtime — in-memory state + JSON-RPC dispatch
// ---------------------------------------------------------------------------

type session struct {
	id, title, model, cwd, createdAt string
}

type runtime struct {
	hub *hub

	mu       sync.Mutex
	seq      int // id minter
	sessions map[string]*session
}

func newRuntime(h *hub) *runtime {
	return &runtime{hub: h, sessions: map[string]*session{}}
}

func (rt *runtime) nextID(prefix string) string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.seq++
	return fmt.Sprintf("%s_%04d", prefix, rt.seq)
}

func serveCwd() string {
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "/"
}

func nowISO() string { return time.Now().UTC().Format(time.RFC3339) }

func infoResponse() map[string]any {
	home, _ := os.UserHomeDir()
	return map[string]any{
		"protocolVersion": ProtocolVersion,
		"serverInfo": map[string]any{
			"name": "lyra-mock", "version": "0.0.0", "cwd": serveCwd(), "home": home,
		},
		"capabilities": capabilities(),
	}
}

func capabilities() map[string]any {
	return map[string]any{
		"protocolVersion": ProtocolVersion,
		"events": []string{
			"run.started", "run.progress", "run.finished",
			"item.started", "item.delta", "item.completed",
			"state.snapshot", "state.delta",
		},
		"features": map[string]any{
			"reasoning": true, "mcp": false, "multimodal": false, "checkpoints": false,
			"background": false, "subagents": false, "skills": false, "sessionExport": false,
			"memory": false, "relocate": true, "clientTools": false,
			"attachments": map[string]any{"enabled": false},
		},
		"providers": []string{},
		"limits":    map[string]any{"maxConcurrentRuns": 8},
	}
}

// dispatchNotification handles client→server notifications (no response).
func (rt *runtime) dispatchNotification(method string, _ json.RawMessage) {
	// runtime.shutdown / notifications.canceled — nothing to do in the mock.
	_ = method
}

// dispatch handles a request method, returning a result or a JSON-RPC error.
func (rt *runtime) dispatch(connID, method string, params json.RawMessage) (any, *rpcError) {
	switch method {
	case "runtime.initialize":
		return infoResponse(), nil
	case "runtime.ping":
		return map[string]any{}, nil

	case "sessions.create":
		var p struct {
			Cwd, Title, Model string
		}
		_ = json.Unmarshal(params, &p)
		return rt.createSession(p.Cwd, p.Title, p.Model), nil
	case "sessions.list":
		return map[string]any{"data": rt.listSessions()}, nil
	case "sessions.get":
		var p struct{ SessionID string }
		_ = json.Unmarshal(params, &p)
		if s := rt.getSession(p.SessionID); s != nil {
			return sessionJSON(s), nil
		}
		return nil, &rpcError{Code: -32002, Message: "session not found",
			Data: map[string]any{"type": "session_not_found"}}

	case "runs.start":
		var p struct {
			SessionID string         `json:"sessionId"`
			Input     []contentBlock `json:"input"`
		}
		_ = json.Unmarshal(params, &p)
		// Field-level validation → invalid_params + errors[] (API.md §8.3).
		var fieldErrs []map[string]any
		if p.SessionID == "" {
			fieldErrs = append(fieldErrs, map[string]any{"field": "sessionId", "detail": "required"})
		}
		if len(p.Input) == 0 {
			fieldErrs = append(fieldErrs, map[string]any{"field": "input", "detail": "must not be empty"})
		}
		if len(fieldErrs) > 0 {
			return nil, &rpcError{Code: -32602, Message: "invalid params",
				Data: map[string]any{"type": "invalid_params", "errors": fieldErrs}}
		}
		runID := rt.nextID("run")
		go rt.scriptRun(connID, p.SessionID, runID, inputText(p.Input))
		return map[string]any{"runId": runID}, nil

	case "runs.resume":
		var p struct {
			ParentRunID string `json:"parentRunId"`
			Responses   []any  `json:"responses"`
		}
		_ = json.Unmarshal(params, &p)
		runID := rt.nextID("run")
		go rt.scriptResume(connID, runID, p.ParentRunID)
		return map[string]any{"runId": runID}, nil

	case "runs.cancel", "runs.subscribe":
		return map[string]any{}, nil
	case "runs.list":
		return []any{}, nil
	case "runs.listOpenInterrupts":
		return []any{}, nil
	case "items.list":
		return map[string]any{"items": []any{}, "runs": []any{}}, nil

	// Workspace + catalog reads — empty in the mock.
	case "workspace.listFileChanges", "workspace.listProjects", "workspace.listSkills",
		"workspace.listAgentDocs", "workspace.mcp.listServers", "workspace.mcp.listTools",
		"providers.list", "models.list", "tools.list":
		return []any{}, nil
	case "workspace.getDiff":
		return []any{}, nil
	case "workspace.grep":
		return map[string]any{"matches": []any{}, "total": 0}, nil

	default:
		return nil, errMethodNotFound(method)
	}
}

// ---------------------------------------------------------------------------
// session store
// ---------------------------------------------------------------------------

func (rt *runtime) createSession(cwd, title, model string) map[string]any {
	if cwd == "" {
		cwd = serveCwd()
	}
	if title == "" {
		title = "New session"
	}
	if model == "" {
		model = "claude-mock"
	}
	s := &session{id: rt.nextID("ses"), title: title, model: model, cwd: cwd, createdAt: nowISO()}
	rt.mu.Lock()
	rt.sessions[s.id] = s
	rt.mu.Unlock()
	return sessionJSON(s)
}

func (rt *runtime) getSession(id string) *session {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.sessions[id]
}

func (rt *runtime) listSessions() []map[string]any {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	out := make([]map[string]any, 0, len(rt.sessions))
	for _, s := range rt.sessions {
		out = append(out, sessionJSON(s))
	}
	return out
}

func sessionJSON(s *session) map[string]any {
	return map[string]any{
		"id": s.id, "title": s.title, "status": "idle", "model": s.model, "cwd": s.cwd,
		"createdAt": s.createdAt, "updatedAt": s.createdAt, "metadata": map[string]any{},
	}
}

// ---------------------------------------------------------------------------
// scripted run streaming
// ---------------------------------------------------------------------------

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func inputText(blocks []contentBlock) string {
	var b strings.Builder
	for _, c := range blocks {
		if c.Type == "text" {
			b.WriteString(c.Text)
		}
	}
	return b.String()
}

// emitter streams RunEvents for one root run to a connection, minting a
// monotonic eventId per frame (API.md §2.2: monotonic within the root stream).
type emitter struct {
	rt     *runtime
	connID string
	runID  string
	n      int
}

func (e *emitter) emit(durable bool, event map[string]any) {
	e.n++
	eventID := fmt.Sprintf("evt_%010d", e.n)
	runEvent := map[string]any{
		"runId": e.runID, "eventId": eventID, "timestamp": nowISO(),
		"durable": durable, "event": event,
	}
	frame, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "method": "notifications.run.event", "params": runEvent,
	})
	e.rt.hub.send(e.connID, notification{eventID: eventID, data: string(frame)})
}

func item(id, runID, typ, status string, extra map[string]any) map[string]any {
	m := map[string]any{"id": id, "runId": runID, "status": status, "createdAt": nowISO(), "type": typ}
	maps.Copy(m, extra)
	return m
}

func textBlocks(s string) []map[string]any {
	return []map[string]any{{"type": "text", "text": s}}
}

// scriptRun streams a believable agent turn. Inputs containing "rm" / "sudo" /
// "delete" branch into an approval interrupt (R-model HITL, §6) so the desktop
// app can exercise the resume flow; everything else is a happy path with a
// streamed message + one tool call.
func (rt *runtime) scriptRun(connID, sessionID, runID, input string) {
	e := &emitter{rt: rt, connID: connID, runID: runID}
	model := "claude-mock"
	if s := rt.getSession(sessionID); s != nil {
		model = s.model
	}
	e.emit(true, map[string]any{"type": "run.started",
		"run": map[string]any{"id": runID, "sessionId": sessionID, "model": model, "mode": "agent"}})

	// Streamed assistant message.
	msgID := rt.nextID("item")
	e.emit(true, map[string]any{"type": "item.started",
		"item": item(msgID, runID, "agentMessage", "inProgress", map[string]any{"content": []any{}})})
	reply := "Sure — here's a quick look at the working tree."
	for _, chunk := range chunkWords(reply) {
		time.Sleep(40 * time.Millisecond)
		e.emit(false, map[string]any{"type": "item.delta", "itemId": msgID,
			"delta": map[string]any{"type": "content", "text": chunk}})
	}
	e.emit(true, map[string]any{"type": "item.completed",
		"item": item(msgID, runID, "agentMessage", "completed", map[string]any{"content": textBlocks(reply)})})

	dangerous := strings.Contains(input, "rm") ||
		strings.Contains(input, "sudo") ||
		strings.Contains(input, "delete")

	toolID := rt.nextID("item")
	cmd := []string{"ls", "-la"}
	if dangerous {
		cmd = []string{"rm", "-rf", "./build"}
	}
	// mid-run progress preview (ephemeral; authoritative totals on run.finished).
	e.emit(false, map[string]any{"type": "run.progress",
		"progress": map[string]any{"step": 1, "maxSteps": 8,
			"activity": "calling tool: " + strings.Join(cmd, " "),
			"usage":    map[string]any{"inputTokens": 120, "outputTokens": 48}}})
	e.emit(true, map[string]any{"type": "item.started",
		"item": item(toolID, runID, "toolCall", "inProgress",
			map[string]any{"tool": map[string]any{"kind": "commandExecution", "command": cmd}})})

	if dangerous {
		// R-model HITL: end the run with an approval interrupt.
		time.Sleep(150 * time.Millisecond)
		e.emit(true, map[string]any{"type": "run.finished", "outcome": map[string]any{
			"type": "interrupt",
			"interrupts": []any{map[string]any{
				"itemId": toolID, "kind": "approval",
				"payload": map[string]any{
					"command": strings.Join(cmd, " "), "text": "Run this command?", "reason": "It deletes files.",
				},
			}},
		}})
		return
	}

	// stdout/stderr stream via toolOutput delta as a live PREVIEW (durable=false),
	// then the authoritative merged `output` settles on item.completed (durable).
	// Dropping the delta still leaves correct output on completed (API.md §5.2,
	// docs/protocol/TOOL_OUTPUT.md).
	const out = "total 8\ndrwxr-xr-x  4 user  staff  128 .\n"
	time.Sleep(120 * time.Millisecond)
	e.emit(false, map[string]any{"type": "item.delta", "itemId": toolID,
		"delta": map[string]any{"type": "toolOutput", "text": out}})
	e.emit(true, map[string]any{"type": "item.completed",
		"item": item(toolID, runID, "toolCall", "completed",
			map[string]any{"tool": map[string]any{"kind": "commandExecution", "command": cmd,
				"output": out, "exitCode": 0}})})

	rt.finishCompleted(e)
}

// scriptResume streams a short continuation Run answering an interrupt.
func (rt *runtime) scriptResume(connID, runID, parentRunID string) {
	e := &emitter{rt: rt, connID: connID, runID: runID}
	e.emit(true, map[string]any{"type": "run.started",
		"run": map[string]any{"id": runID, "parentRunId": parentRunID}})
	msgID := rt.nextID("item")
	reply := "Done — the command finished successfully."
	time.Sleep(80 * time.Millisecond)
	e.emit(true, map[string]any{"type": "item.completed",
		"item": item(msgID, runID, "agentMessage", "completed", map[string]any{"content": textBlocks(reply)})})
	rt.finishCompleted(e)
}

func (rt *runtime) finishCompleted(e *emitter) {
	e.emit(true, map[string]any{"type": "run.finished", "outcome": map[string]any{
		"type": "completed",
		"result": map[string]any{
			"usage": map[string]any{"inputTokens": 120, "outputTokens": 48}, "steps": 1,
		},
	}})
}

// chunkWords splits text into word-ish chunks so content streams visibly.
func chunkWords(s string) []string {
	parts := strings.SplitAfter(s, " ")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
