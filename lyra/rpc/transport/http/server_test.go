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
	lyrahttp "github.com/Tangerg/lynx/lyra/rpc/transport/http"
)

const testProtocolVersion = "2026-06-03"

// fakeRuntime is the smallest Runtime we can pass to NewServer for
// smoke-testing the transport layer. The embedded nil protocol.Runtime
// supplies the methods the tests don't exercise (they panic if hit);
// the tests only touch lifecycle, sidecars, error paths, runs.cancel.
type fakeRuntime struct {
	protocol.Runtime
	cancelledRuns []string
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

// TestSidecarHealth confirms /v2/health returns 200 + {"status":"ok"}
// and never enforces auth.
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
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
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

	reqBody := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.initialize","params":{"protocolVersion":"2026-06-03","clientInfo":{"name":"test","version":"0"},"capabilities":{"events":[]}}}`)
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
// returns 409 + invalid_request (-32600).
func TestURLBodyMethodMismatch(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	body := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.ping"}`)
	resp, err := netHTTP.Post(ts.URL+"/v2/rpc/runtime.initialize", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 409 {
		raw := readBody(resp)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, raw)
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
