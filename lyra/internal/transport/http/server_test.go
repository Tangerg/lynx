package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"iter"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	chatmodel "github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/lyra/internal/engine"
	"github.com/Tangerg/lynx/lyra/internal/service/approval"
	"github.com/Tangerg/lynx/lyra/internal/service/chat"
	"github.com/Tangerg/lynx/lyra/internal/service/session"
	lyrahttp "github.com/Tangerg/lynx/lyra/internal/transport/http"
)

// TestHealthz is the smoke test — the server boots and replies
// 200 on the liveness probe.
func TestHealthz(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

// TestSessionsCRUD exercises create → list → get → delete on the
// session endpoints, verifying both the happy path and the
// 404-on-missing behaviour.
func TestSessionsCRUD(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	// Create with a title
	createBody := bytes.NewBufferString(`{"title":"http test"}`)
	resp := mustDo(t, http.MethodPost, ts.URL+"/v1/sessions", createBody)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d", resp.StatusCode)
	}
	var created map[string]any
	mustDecode(t, resp.Body, &created)
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatal("create returned empty id")
	}

	// List should include the newly created session
	resp = mustDo(t, http.MethodGet, ts.URL+"/v1/sessions", nil)
	var listed map[string][]map[string]any
	mustDecode(t, resp.Body, &listed)
	if len(listed["sessions"]) != 1 || listed["sessions"][0]["id"] != id {
		t.Fatalf("list = %+v", listed)
	}

	// Get one
	resp = mustDo(t, http.MethodGet, ts.URL+"/v1/sessions/"+id, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get status = %d", resp.StatusCode)
	}

	// Get unknown → 404
	resp = mustDo(t, http.MethodGet, ts.URL+"/v1/sessions/no-such", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing get status = %d, want 404", resp.StatusCode)
	}

	// Delete the real one
	resp = mustDo(t, http.MethodDelete, ts.URL+"/v1/sessions/"+id, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d", resp.StatusCode)
	}
}

// TestAgentRunStreamsSSE drives /v1/agent/run end-to-end: the
// server must auto-create a session (empty threadId), drive a turn
// against the stub model, and emit a valid AG-UI SSE stream. We
// assert on the canonical event-type ordering — RUN_STARTED first,
// RUN_FINISHED last, with tool + text events in between.
func TestAgentRunStreamsSSE(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	body := bytes.NewBufferString(`{"message":"say lyra via bash"}`)
	resp, err := http.Post(ts.URL+"/v1/agent/run", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	events := parseSSEEventTypes(string(raw))
	if len(events) == 0 {
		t.Fatalf("got no SSE events; body = %q", string(raw))
	}
	if events[0] != "RUN_STARTED" {
		t.Errorf("first event = %q, want RUN_STARTED (full: %v)", events[0], events)
	}
	last := events[len(events)-1]
	if last != "RUN_FINISHED" && last != "RUN_ERROR" {
		t.Errorf("last event = %q, want RUN_FINISHED|RUN_ERROR (full: %v)", last, events)
	}
}

// TestSteerEndpoint_404OnUnknownTurn exercises the steer endpoint
// against an unknown turn id — the underlying service returns
// ErrTurnNotFound which the transport must map to 404.
func TestSteerEndpoint_404OnUnknownTurn(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	body := bytes.NewBufferString(`{"message":"steer me"}`)
	resp := mustDo(t, http.MethodPost, ts.URL+"/v1/turns/no-such/steer", body)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

// TestSteerEndpoint_BadJSON returns 400 on malformed body.
func TestSteerEndpoint_BadJSON(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	body := bytes.NewBufferString(`{not json`)
	resp := mustDo(t, http.MethodPost, ts.URL+"/v1/turns/whatever/steer", body)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

// TestApprovalModeGetSet round-trips the runtime approval mode
// through the HTTP surface. POST changes it, GET observes the
// change.
func TestApprovalModeGetSet(t *testing.T) {
	ts := newTestServerWithApproval(t, approval.New(approval.ModeYolo))
	defer ts.Close()

	resp := mustDo(t, http.MethodGet, ts.URL+"/v1/approvals/mode", nil)
	var got modeWire
	mustDecode(t, resp.Body, &got)
	if got.Mode != "yolo" {
		t.Errorf("initial mode = %q, want yolo", got.Mode)
	}

	body := bytes.NewBufferString(`{"mode":"safe"}`)
	resp = mustDo(t, http.MethodPost, ts.URL+"/v1/approvals/mode", body)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("set status = %d, want 204", resp.StatusCode)
	}

	resp = mustDo(t, http.MethodGet, ts.URL+"/v1/approvals/mode", nil)
	mustDecode(t, resp.Body, &got)
	if got.Mode != "safe" {
		t.Errorf("after set, mode = %q, want safe", got.Mode)
	}
}

// TestApprovalModeBadValue returns 400 on unknown mode strings.
func TestApprovalModeBadValue(t *testing.T) {
	ts := newTestServerWithApproval(t, approval.New(approval.ModeYolo))
	defer ts.Close()

	body := bytes.NewBufferString(`{"mode":"reckless"}`)
	resp := mustDo(t, http.MethodPost, ts.URL+"/v1/approvals/mode", body)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

// TestApprovalEndpointsRequireService returns 503 when the
// server was started without an approval service wired.
func TestApprovalEndpointsRequireService(t *testing.T) {
	ts, _ := newTestServer(t) // no approval service
	defer ts.Close()

	resp := mustDo(t, http.MethodGet, ts.URL+"/v1/approvals/mode", nil)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", resp.StatusCode)
	}
}

// modeWire mirrors http.modeBody so the tests stay decoupled
// from the unexported request/response struct.
type modeWire struct {
	Mode string `json:"mode"`
}

// ------------------------------------------------------------------
// Test harness
// ------------------------------------------------------------------

// newTestServer wires a stub-backed Lyra runtime (in-memory chat
// + session + tool-call stub model) behind an httptest server, so
// every endpoint runs through the same dispatch chain a real
// `lyra serve` would use.
func newTestServer(t *testing.T) (*httptest.Server, chat.Service) {
	t.Helper()

	client, err := chatmodel.NewClient(newStubChatModel())
	if err != nil {
		t.Fatalf("chat client: %v", err)
	}
	eng, err := engine.New(engine.Config{ChatClient: client})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	chatSvc := chat.New(eng, nil)
	sessSvc := session.NewInMemoryService()

	srv, err := lyrahttp.NewServer(lyrahttp.Config{
		Chat:    chatSvc,
		Session: sessSvc,
		Addr:    ":0", // unused; we use Handler() directly
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	ts := httptest.NewServer(srv.Handler())
	return ts, chatSvc
}

// newTestServerWithApproval is the variant used by /v1/approvals
// tests — wires a real approval.Service through the server so
// the mode endpoints behave like production.
func newTestServerWithApproval(t *testing.T, approvalSvc approval.Service) *httptest.Server {
	t.Helper()

	client, _ := chatmodel.NewClient(newStubChatModel())
	eng, _ := engine.New(engine.Config{ChatClient: client})
	chatSvc := chat.New(eng, approvalSvc)
	sessSvc := session.NewInMemoryService()

	srv, err := lyrahttp.NewServer(lyrahttp.Config{
		Chat:     chatSvc,
		Session:  sessSvc,
		Approval: approvalSvc,
		Addr:     ":0",
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return httptest.NewServer(srv.Handler())
}

func mustDo(t *testing.T, method, url string, body io.Reader) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), method, url, body)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func mustDecode(t *testing.T, r io.Reader, v any) {
	t.Helper()
	if err := json.NewDecoder(r).Decode(v); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

// parseSSEEventTypes pulls the AG-UI `type` field out of each
// `data: {...}` SSE frame in raw. Lossy on purpose — the test
// only cares about the ordering of event types, not their
// payloads.
func parseSSEEventTypes(raw string) []string {
	var out []string
	for _, line := range strings.Split(raw, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var payload struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &payload); err != nil {
			continue
		}
		if payload.Type != "" {
			out = append(out, payload.Type)
		}
	}
	return out
}

// ------------------------------------------------------------------
// Stub LLM — duplicated from chat/impl_test.go to keep this
// package self-contained.
// ------------------------------------------------------------------

type stubChatModel struct{ defaults *chatmodel.Options }

func newStubChatModel() *stubChatModel {
	opts, _ := chatmodel.NewOptions("stub-model")
	return &stubChatModel{defaults: opts}
}

func (m *stubChatModel) DefaultOptions() chatmodel.Options { return *m.defaults }
func (m *stubChatModel) Metadata() chatmodel.ModelMetadata { return chatmodel.ModelMetadata{Provider: "stub"} }

func (m *stubChatModel) Call(_ context.Context, req *chatmodel.Request) (*chatmodel.Response, error) {
	if hasToolMsg(req.Messages) {
		return makeText("I ran echo and got lyra.")
	}
	return makeToolCall("bash", `{"command":"echo lyra"}`)
}

func (m *stubChatModel) Stream(ctx context.Context, req *chatmodel.Request) iter.Seq2[*chatmodel.Response, error] {
	resp, err := m.Call(ctx, req)
	return func(yield func(*chatmodel.Response, error) bool) { yield(resp, err) }
}

func hasToolMsg(messages []chatmodel.Message) bool {
	for _, msg := range messages {
		if msg.Type() == chatmodel.MessageTypeTool {
			return true
		}
	}
	return false
}

func makeText(text string) (*chatmodel.Response, error) {
	return chatmodel.NewResponse(
		&chatmodel.Result{
			AssistantMessage: chatmodel.NewAssistantMessage(text),
			Metadata:         &chatmodel.ResultMetadata{FinishReason: chatmodel.FinishReasonStop},
		},
		&chatmodel.ResponseMetadata{},
	)
}

func makeToolCall(name, args string) (*chatmodel.Response, error) {
	calls := []*chatmodel.ToolCallPart{{ID: "c1", Name: name, Arguments: args}}
	return chatmodel.NewResponse(
		&chatmodel.Result{
			AssistantMessage: chatmodel.NewAssistantMessage(calls),
			Metadata:         &chatmodel.ResultMetadata{FinishReason: chatmodel.FinishReasonToolCalls},
		},
		&chatmodel.ResponseMetadata{},
	)
}
