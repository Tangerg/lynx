package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	netHTTP "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/lyra/rpc/protocol"
	lyratransport "github.com/Tangerg/lynx/lyra/rpc/transport"
	lyrahttp "github.com/Tangerg/lynx/lyra/rpc/transport/http"
)

const testProtocolVersion = "2026-06-07"

// fakeRuntime is the smallest Runtime we can pass to NewServer for
// smoke-testing the transport layer. The embedded nil protocol.Runtime
// supplies the methods the tests don't exercise (they panic if hit);
// the tests only touch lifecycle, sidecars, error paths, runs.cancel.
type fakeRuntime struct {
	protocol.Runtime
	cancelledRuns []string
	gotLastEventID string
}

func (f *fakeRuntime) Initialize(_ context.Context, _ protocol.InitializeRequest) (*protocol.InitializeResponse, error) {
	return &protocol.InitializeResponse{ProtocolVersion: testProtocolVersion}, nil
}

func (f *fakeRuntime) Ping(_ context.Context) error { return nil }

func (f *fakeRuntime) CancelRun(_ context.Context, in protocol.CancelRunRequest) error {
	f.cancelledRuns = append(f.cancelledRuns, in.RunID)
	return nil
}

func newTestServer(t *testing.T) (*httptest.Server, *fakeRuntime) {
	t.Helper()
	api := &fakeRuntime{}
	srv, err := lyrahttp.NewServer(lyrahttp.Config{
		Runtime:         api,
		Addr:            ":0",
		ServerInfo:      protocol.ServerInfo{Name: "lyra-test", Version: "0.0.0"},
		ProtocolVersion: testProtocolVersion,
		Capabilities:    protocol.ServerCapabilities{},
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return httptest.NewServer(srv.Handler()), api
}

// TestSidecarInfo confirms /v2/info returns a flat JSON snapshot (NOT a
// JSON-RPC envelope) and surfaces serverInfo + protocolVersion.
func TestSidecarInfo(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	resp, err := netHTTP.Get(ts.URL + "/v2/info")
	if err != nil {
		t.Fatalf("get info: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body struct {
		ServerInfo      protocol.ServerInfo `json:"serverInfo"`
		ProtocolVersion string              `json:"protocolVersion"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.ProtocolVersion != testProtocolVersion {
		t.Fatalf("protocolVersion = %q", body.ProtocolVersion)
	}
	if body.ServerInfo.Name != "lyra-test" {
		t.Fatalf("serverInfo.Name = %q", body.ServerInfo.Name)
	}
}

// TestSidecarHealth confirms /v2/health returns 200 + the contract body
// `{"ok":true}` (TRANSPORT §12.1) and never enforces auth.
func TestSidecarHealth(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	resp, err := netHTTP.Get(ts.URL + "/v2/health")
	if err != nil {
		t.Fatalf("get health: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body struct {
		OK     bool   `json:"ok"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.OK {
		t.Fatalf("ok = false, want true")
	}
	if body.Status != "ok" {
		t.Fatalf("status = %q", body.Status)
	}
}

// TestInitializeOverRPC confirms POST /v2/rpc/runtime.initialize handles
// the request and wraps the result in a JSON-RPC envelope.
func TestInitializeOverRPC(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	reqBody := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.initialize","params":{"protocolVersion":"2026-06-07","clientInfo":{"name":"test","version":"0"},"capabilities":{"events":[]}}}`)
	resp, err := netHTTP.Post(ts.URL+"/v2/rpc/runtime.initialize", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("post rpc: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var env struct {
		JSONRPC string           `json:"jsonrpc"`
		Result  json.RawMessage  `json:"result"`
		Error   *json.RawMessage `json:"error,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.JSONRPC != "2.0" {
		t.Fatalf("jsonrpc = %q", env.JSONRPC)
	}
	if env.Error != nil {
		t.Fatalf("got error envelope: %s", string(*env.Error))
	}
	if !strings.Contains(string(env.Result), testProtocolVersion) {
		t.Fatalf("result missing protocolVersion: %s", string(env.Result))
	}
}

// TestURLPathMethodForm confirms POST /v2/rpc/{method} works and the
// X-Method response header echoes the method.
func TestURLPathMethodForm(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	initBody := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.initialize","params":{}}`)
	initResp, err := netHTTP.Post(ts.URL+"/v2/rpc/runtime.initialize", "application/json", bytes.NewReader(initBody))
	if err != nil {
		t.Fatalf("post initialize: %v", err)
	}
	initResp.Body.Close()
	if initResp.StatusCode != 200 {
		t.Fatalf("initialize status = %d", initResp.StatusCode)
	}
	if got := initResp.Header.Get("X-Method"); got != "runtime.initialize" {
		t.Fatalf("X-Method = %q, want runtime.initialize", got)
	}

	pingBody := []byte(`{"jsonrpc":"2.0","id":"2","method":"runtime.ping"}`)
	pingResp, err := netHTTP.Post(ts.URL+"/v2/rpc/runtime.ping", "application/json", bytes.NewReader(pingBody))
	if err != nil {
		t.Fatalf("post ping: %v", err)
	}
	defer pingResp.Body.Close()
	if pingResp.StatusCode != 200 {
		t.Fatalf("ping status = %d", pingResp.StatusCode)
	}
	if got := pingResp.Header.Get("X-Method"); got != "runtime.ping" {
		t.Fatalf("X-Method = %q, want runtime.ping", got)
	}
}

// TestURLBodyMethodMismatch confirms the URL/body method-mismatch guard
// returns 400 + invalid_request (-32600) — a self-contradictory malformed
// request, NOT a 409 resource conflict (TRANSPORT §6.2/§6.3).
func TestURLBodyMethodMismatch(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	body := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.ping"}`)
	resp, err := netHTTP.Post(ts.URL+"/v2/rpc/runtime.initialize", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		raw := readBody(resp)
		t.Fatalf("status = %d, want 400; body = %s", resp.StatusCode, raw)
	}
	if code := decodeErrorCode(t, resp); code != -32600 {
		t.Fatalf("expected -32600, got %d", code)
	}
}

// TestUnknownMethodReturns404 confirms a typo in the URL form surfaces
// as 404 + method_not_found (-32601).
func TestUnknownMethodReturns404(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	initBody := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.initialize","params":{}}`)
	r1, _ := netHTTP.Post(ts.URL+"/v2/rpc/runtime.initialize", "application/json", bytes.NewReader(initBody))
	r1.Body.Close()

	body := []byte(`{"jsonrpc":"2.0","id":"2","method":"runs.unknownMethod"}`)
	resp, err := netHTTP.Post(ts.URL+"/v2/rpc/runs.unknownMethod", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if code := decodeErrorCode(t, resp); code != -32601 {
		t.Fatalf("expected -32601, got %d", code)
	}
}

// TestCapabilityGateBeforeInitialize confirms a business method called
// before runtime.initialize gets capability_not_negotiated (-32006).
func TestCapabilityGateBeforeInitialize(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	body := []byte(`{"jsonrpc":"2.0","id":"1","method":"sessions.list","params":{}}`)
	resp, err := netHTTP.Post(ts.URL+"/v2/rpc/sessions.list", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if code := decodeErrorCode(t, resp); code != -32006 {
		t.Fatalf("expected -32006, got %d", code)
	}
}

// TestRPCWithoutMethodReturns404 confirms POST to /v2/rpc without a
// {method} suffix is unrouted — 404, no JSON-RPC envelope.
func TestRPCWithoutMethodReturns404(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	body := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.initialize","params":{}}`)
	resp, err := netHTTP.Post(ts.URL+"/v2/rpc", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		raw := readBody(resp)
		t.Fatalf("status = %d, want 404; body = %s", resp.StatusCode, raw)
	}
}

// TestNonStringIDRejected confirms API.md §2.2: the envelope id must be
// a STRING; a numeric id is rejected with invalid_request (-32600).
func TestNonStringIDRejected(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	body := []byte(`{"jsonrpc":"2.0","id":42,"method":"runtime.initialize","params":{}}`)
	resp, err := netHTTP.Post(ts.URL+"/v2/rpc/runtime.initialize", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		raw := readBody(resp)
		t.Fatalf("status = %d, want 400; body = %s", resp.StatusCode, raw)
	}
	if code := decodeErrorCode(t, resp); code != -32600 {
		t.Fatalf("expected -32600, got %d", code)
	}
}

// TestRunsCancelIsRequest confirms runs.cancel is a proper Request
// method (not a notification): 200 + JSON-RPC envelope, NOT 204.
func TestRunsCancelIsRequest(t *testing.T) {
	ts, api := newTestServer(t)
	defer ts.Close()

	initBody := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.initialize","params":{}}`)
	r1, err := netHTTP.Post(ts.URL+"/v2/rpc/runtime.initialize", "application/json", bytes.NewReader(initBody))
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	r1.Body.Close()

	body := []byte(`{"jsonrpc":"2.0","id":"2","method":"runs.cancel","params":{"runId":"run_123"}}`)
	resp, err := netHTTP.Post(ts.URL+"/v2/rpc/runs.cancel", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		raw := readBody(resp)
		t.Fatalf("status = %d, want 200; body = %s", resp.StatusCode, raw)
	}
	var env struct {
		JSONRPC string           `json:"jsonrpc"`
		ID      json.RawMessage  `json:"id"`
		Result  json.RawMessage  `json:"result"`
		Error   *json.RawMessage `json:"error,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Error != nil {
		t.Fatalf("got error envelope: %s", string(*env.Error))
	}
	if string(env.ID) != `"2"` {
		t.Fatalf("id = %s, want \"2\"", string(env.ID))
	}
	if len(api.cancelledRuns) != 1 || api.cancelledRuns[0] != "run_123" {
		t.Fatalf("api.cancelledRuns = %v, want [run_123]", api.cancelledRuns)
	}
}

// TestNotificationReturns204 confirms a client→server notification (no
// envelope id) is acknowledged with 204 No Content and no response body
// (TRANSPORT §6.3 picks 204 over 202 — dispatch is already complete).
func TestNotificationReturns204(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	initBody := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.initialize","params":{}}`)
	r1, _ := netHTTP.Post(ts.URL+"/v2/rpc/runtime.initialize", "application/json", bytes.NewReader(initBody))
	r1.Body.Close()

	// notifications.canceled has no id ⇒ it's a Notification; JSON-RPC
	// never sends a response for one, so the transport acks with 204.
	body := []byte(`{"jsonrpc":"2.0","method":"notifications.canceled","params":{"id":"1"}}`)
	resp, err := netHTTP.Post(ts.URL+"/v2/rpc/notifications.canceled", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 204 {
		raw := readBody(resp)
		t.Fatalf("status = %d, want 204; body = %s", resp.StatusCode, raw)
	}
}

// TestBodyTooLargeReturns413 confirms an oversized POST body is rejected
// with 413 (TRANSPORT §6.3) rather than silently truncated into a parse
// error.
func TestBodyTooLargeReturns413(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	big := bytes.Repeat([]byte("a"), (4<<20)+1)
	resp, err := netHTTP.Post(ts.URL+"/v2/rpc/runtime.initialize", "application/json", bytes.NewReader(big))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 413 {
		t.Fatalf("status = %d, want 413", resp.StatusCode)
	}
}

// TestUnsupportedMediaTypeReturns415 confirms a non-JSON Content-Type is
// rejected with 415 (TRANSPORT §6.3).
func TestUnsupportedMediaTypeReturns415(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	body := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.initialize","params":{}}`)
	resp, err := netHTTP.Post(ts.URL+"/v2/rpc/runtime.initialize", "text/plain", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 415 {
		t.Fatalf("status = %d, want 415", resp.StatusCode)
	}
}

// TestMethodNotAllowedHasAllow confirms a wrong HTTP method on a known
// endpoint returns 405 with an Allow header listing the supported methods
// (RFC 9110 §15.5.6 / TRANSPORT §6.3). chi populates Allow from the route.
func TestMethodNotAllowedHasAllow(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	// /v2/rpc/{method} is POST-only; a GET to it is 405, not 404.
	req, _ := netHTTP.NewRequest("GET", ts.URL+"/v2/rpc/runtime.ping", nil)
	resp, err := netHTTP.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 405 {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
	if allow := resp.Header.Get("Allow"); !strings.Contains(allow, "POST") {
		t.Fatalf("Allow = %q, must list POST", allow)
	}
}

// decodeErrorCode reads a JSON-RPC error envelope and returns its code.
func decodeErrorCode(t *testing.T, resp *netHTTP.Response) int {
	t.Helper()
	var env struct {
		Error *struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Error == nil {
		t.Fatalf("expected an error envelope, got none")
	}
	return env.Error.Code
}

// readBody reads the response body into a string for diagnostic
// t.Fatalf messages.
func readBody(r *netHTTP.Response) string {
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(r.Body)
	return buf.String()
}

// StartRun lets the fake drive the streamable path: it returns a runId
// ack plus a pre-baked, pre-closed RunEvent channel (run.started →
// run.finished) so a POST runs.start exercises serveStream end-to-end.
func (f *fakeRuntime) StartRun(_ context.Context, in protocol.StartRunRequest) (*protocol.StartRunResponse, <-chan protocol.RunEvent, error) {
	ch := make(chan protocol.RunEvent, 2)
	ch <- protocol.RunEvent{RunID: "run_x", EventID: "evt_00000000001", Durable: true,
		Event: protocol.StreamEvent{Type: protocol.StreamRunStarted, Run: &protocol.RunRef{ID: "run_x", SessionID: in.SessionID}}}
	ch <- protocol.RunEvent{RunID: "run_x", EventID: "evt_00000000002", Durable: true,
		Event: protocol.StreamEvent{Type: protocol.StreamRunFinished, Outcome: &protocol.RunOutcome{Type: protocol.OutcomeCompleted, Result: &protocol.RunResult{}}}}
	close(ch)
	return &protocol.StartRunResponse{RunID: "run_x"}, ch, nil
}

type sseFrame struct{ id, data string }

// parseSSE splits a text/event-stream body into frames, lifting the id:
// and data: lines and skipping comments / blanks.
func parseSSE(raw string) []sseFrame {
	var out []sseFrame
	for _, block := range strings.Split(strings.TrimSpace(raw), "\n\n") {
		var f sseFrame
		hasData := false
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "id:"):
				f.id = strings.TrimSpace(line[len("id:"):])
			case strings.HasPrefix(line, "data:"):
				f.data += strings.TrimSpace(line[len("data:"):])
				hasData = true
			}
		}
		if hasData {
			out = append(out, f)
		}
	}
	return out
}

// TestStreamableRunStart confirms a streaming method's POST response is
// itself the event stream (TRANSPORT §6.4): 200 text/event-stream, first
// frame = the JSON-RPC ack (runId, no SSE id), then run-event frames each
// carrying SSE id: = eventId.
func TestStreamableRunStart(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	initBody := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.initialize","params":{}}`)
	r0, _ := netHTTP.Post(ts.URL+"/v2/rpc/runtime.initialize", "application/json", bytes.NewReader(initBody))
	r0.Body.Close()

	body := []byte(`{"jsonrpc":"2.0","id":"2","method":"runs.start","params":{"sessionId":"ses_1","input":[{"type":"text","text":"hi"}]}}`)
	req, _ := netHTTP.NewRequest("POST", ts.URL+"/v2/rpc/runs.start", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := netHTTP.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}

	frames := parseSSE(readBody(resp))
	if len(frames) != 3 {
		t.Fatalf("frames = %d, want 3 (ack + started + finished)", len(frames))
	}
	if frames[0].id != "" || !strings.Contains(frames[0].data, `"runId":"run_x"`) {
		t.Fatalf("ack frame = %+v, want runId result with no SSE id", frames[0])
	}
	if frames[1].id != "evt_00000000001" || !strings.Contains(frames[1].data, "run.started") {
		t.Fatalf("frame[1] = %+v, want run.started @ evt 1", frames[1])
	}
	if frames[2].id != "evt_00000000002" || !strings.Contains(frames[2].data, "run.finished") {
		t.Fatalf("frame[2] = %+v, want run.finished @ evt 2", frames[2])
	}
}

// gotLastEventID records the reconnect cursor SubscribeRun observed, so
// the test below can assert the transport plumbed the Last-Event-Id
// header onto the dispatch ctx (TRANSPORT §9.2).
func (f *fakeRuntime) SubscribeRun(ctx context.Context, runID string) (*protocol.StartRunResponse, <-chan protocol.RunEvent, error) {
	f.gotLastEventID = lyratransport.LastEventIDFrom(ctx)
	ch := make(chan protocol.RunEvent)
	close(ch) // immediate end-of-stream; the test only checks the cursor
	return &protocol.StartRunResponse{RunID: runID}, ch, nil
}

// TestSubscribeCarriesLastEventID confirms the transport lifts the
// Last-Event-Id request header onto the ctx so runs.subscribe resumes
// from it instead of full-replaying (TRANSPORT §9.2).
func TestSubscribeCarriesLastEventID(t *testing.T) {
	ts, api := newTestServer(t)
	defer ts.Close()

	initBody := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.initialize","params":{}}`)
	r0, _ := netHTTP.Post(ts.URL+"/v2/rpc/runtime.initialize", "application/json", bytes.NewReader(initBody))
	r0.Body.Close()

	body := []byte(`{"jsonrpc":"2.0","id":"2","method":"runs.subscribe","params":{"runId":"run_1"}}`)
	req, _ := netHTTP.NewRequest("POST", ts.URL+"/v2/rpc/runs.subscribe", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Last-Event-Id", "evt_00000000042")
	resp, err := netHTTP.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	resp.Body.Close()

	if api.gotLastEventID != "evt_00000000042" {
		t.Fatalf("SubscribeRun saw Last-Event-Id %q, want evt_00000000042", api.gotLastEventID)
	}
}
