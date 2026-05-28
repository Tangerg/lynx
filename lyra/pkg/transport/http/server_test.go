package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	netHTTP "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/lyra/pkg/coreapi"
	lyrahttp "github.com/Tangerg/lynx/lyra/pkg/transport/http"
)

// fakeAPI is the smallest CoreAPI we can pass to NewServer for
// smoke-testing the transport layer. Anything we don't exercise is
// allowed to panic — the tests only hit lifecycle, sidecars, error
// paths.
type fakeAPI struct {
	coreapi.CoreAPI
	cancelledRuns []string
}

func (f *fakeAPI) Initialize(_ context.Context, _ coreapi.InitializeIn) (*coreapi.InitializeOut, error) {
	return &coreapi.InitializeOut{ProtocolVersion: "2026-05-28"}, nil
}

func (f *fakeAPI) Ping(_ context.Context) error { return nil }

func (f *fakeAPI) CancelRun(_ context.Context, runID string) error {
	f.cancelledRuns = append(f.cancelledRuns, runID)
	return nil
}

func newTestServer(t *testing.T) (*httptest.Server, *fakeAPI) {
	t.Helper()
	api := &fakeAPI{}
	srv, err := lyrahttp.NewServer(lyrahttp.Config{
		API:             api,
		Addr:            ":0",
		ServerInfo:      coreapi.ServerInfo{Name: "lyra-test", Version: "0.0.0"},
		ProtocolVersion: "2026-05-28",
		Capabilities:    coreapi.ServerCapabilities{},
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return httptest.NewServer(srv.Handler()), api
}

// TestSidecarInfo confirms /v1/info returns a flat JSON snapshot
// (NOT a JSON-RPC envelope) and surfaces serverInfo + protocolVersion.
// API.md §9.2 — this is the oncall-friendliness contract.
func TestSidecarInfo(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	resp, err := netHTTP.Get(ts.URL + "/v1/info")
	if err != nil {
		t.Fatalf("get info: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body struct {
		ServerInfo      coreapi.ServerInfo `json:"serverInfo"`
		ProtocolVersion string             `json:"protocolVersion"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.ProtocolVersion != "2026-05-28" {
		t.Fatalf("protocolVersion = %q", body.ProtocolVersion)
	}
	if body.ServerInfo.Name != "lyra-test" {
		t.Fatalf("serverInfo.Name = %q", body.ServerInfo.Name)
	}
}

// TestSidecarHealth confirms /v1/health returns 200 + {"status":"ok"}
// and never enforces auth.
func TestSidecarHealth(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	resp, err := netHTTP.Get(ts.URL + "/v1/health")
	if err != nil {
		t.Fatalf("get health: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body struct{ Status string `json:"status"` }
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "ok" {
		t.Fatalf("status = %q", body.Status)
	}
}

// TestInitializeOverRPC confirms POST /v1/rpc/runtime.initialize handles
// a runtime.initialize request and returns the result wrapped in a
// JSON-RPC envelope. API.md v4 §10.1 mandates the URL-method form;
// requests to /v1/rpc without a method suffix get 404 (see
// TestRPCWithoutMethodReturns404).
func TestInitializeOverRPC(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	reqBody := []byte(`{"jsonrpc":"2.0","id":1,"method":"runtime.initialize","params":{"protocolVersion":"2026-05-28","clientInfo":{"name":"test","version":"0"},"capabilities":{"events":{"standard":[],"custom":[]},"features":{}}}}`)
	resp, err := netHTTP.Post(ts.URL+"/v1/rpc/runtime.initialize", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("post rpc: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var env struct {
		JSONRPC string          `json:"jsonrpc"`
		Result  json.RawMessage `json:"result"`
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
	if !strings.Contains(string(env.Result), "2026-05-28") {
		t.Fatalf("result missing protocolVersion: %s", string(env.Result))
	}
}

// TestURLPathMethodForm confirms POST /v1/rpc/{method} works and
// the X-Lyra-Method response header echoes the method.
func TestURLPathMethodForm(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	// initialize first so subsequent calls don't trip the gate
	initBody := []byte(`{"jsonrpc":"2.0","id":1,"method":"runtime.initialize","params":{}}`)
	initResp, err := netHTTP.Post(ts.URL+"/v1/rpc/runtime.initialize", "application/json", bytes.NewReader(initBody))
	if err != nil {
		t.Fatalf("post initialize: %v", err)
	}
	initResp.Body.Close()
	if initResp.StatusCode != 200 {
		t.Fatalf("initialize status = %d", initResp.StatusCode)
	}
	if got := initResp.Header.Get("X-Lyra-Method"); got != "runtime.initialize" {
		t.Fatalf("X-Lyra-Method = %q, want runtime.initialize", got)
	}

	pingBody := []byte(`{"jsonrpc":"2.0","id":2,"method":"runtime.ping"}`)
	pingResp, err := netHTTP.Post(ts.URL+"/v1/rpc/runtime.ping", "application/json", bytes.NewReader(pingBody))
	if err != nil {
		t.Fatalf("post ping: %v", err)
	}
	defer pingResp.Body.Close()
	if pingResp.StatusCode != 200 {
		t.Fatalf("ping status = %d", pingResp.StatusCode)
	}
	if got := pingResp.Header.Get("X-Lyra-Method"); got != "runtime.ping" {
		t.Fatalf("X-Lyra-Method = %q, want runtime.ping", got)
	}
}

// TestURLBodyMethodMismatch confirms the protocol-violation guard
// returns 409 + -32011 when URL and body disagree.
func TestURLBodyMethodMismatch(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"runtime.ping"}`)
	resp, err := netHTTP.Post(ts.URL+"/v1/rpc/runtime.initialize", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 409 {
		raw := readBody(resp)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, raw)
	}
	var env struct {
		Error *struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Error == nil || env.Error.Code != -32011 {
		t.Fatalf("expected -32011, got envelope %+v", env)
	}
}

// TestUnknownMethodReturns404 confirms a typo in the URL form
// surfaces as 404 + -32601 (API.md §7.3).
func TestUnknownMethodReturns404(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	// initialize first so the gate doesn't fire
	initBody := []byte(`{"jsonrpc":"2.0","id":1,"method":"runtime.initialize","params":{}}`)
	r1, _ := netHTTP.Post(ts.URL+"/v1/rpc/runtime.initialize", "application/json", bytes.NewReader(initBody))
	r1.Body.Close()

	body := []byte(`{"jsonrpc":"2.0","id":2,"method":"runs.unknownMethod"}`)
	resp, err := netHTTP.Post(ts.URL+"/v1/rpc/runs.unknownMethod", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var env struct {
		Error *struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Error == nil || env.Error.Code != -32601 {
		t.Fatalf("expected -32601, got %+v", env)
	}
}

// TestProtocolGateBeforeInitialize confirms a business method called
// before runtime.initialize gets -32011 protocol_violation.
func TestProtocolGateBeforeInitialize(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"sessions.list","params":{}}`)
	resp, err := netHTTP.Post(ts.URL+"/v1/rpc/sessions.list", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	var env struct {
		Error *struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Error == nil || env.Error.Code != -32011 {
		t.Fatalf("expected -32011, got %+v", env)
	}
}

// TestRPCWithoutMethodReturns404 confirms v4 §10.1 mandate: POST to
// /v1/rpc without a {method} suffix is unrouted — the server returns
// 404 with no JSON-RPC envelope (greenfield, no fallback path).
func TestRPCWithoutMethodReturns404(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"runtime.initialize","params":{}}`)
	resp, err := netHTTP.Post(ts.URL+"/v1/rpc", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		raw := readBody(resp)
		t.Fatalf("status = %d, want 404; body = %s", resp.StatusCode, raw)
	}
}

// TestNonNumberIDRejected confirms API.md v4 §1.1: JSON-RPC id must
// be a number, string ids are rejected at the dispatcher boundary
// with -32600 invalid_request.
func TestNonNumberIDRejected(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	body := []byte(`{"jsonrpc":"2.0","id":"not-a-number","method":"runtime.initialize","params":{}}`)
	resp, err := netHTTP.Post(ts.URL+"/v1/rpc/runtime.initialize", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		raw := readBody(resp)
		t.Fatalf("status = %d, want 400; body = %s", resp.StatusCode, raw)
	}
	var env struct {
		Error *struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Error == nil || env.Error.Code != -32600 {
		t.Fatalf("expected -32600, got %+v", env)
	}
}

// TestRunsCancelIsRequest confirms API.md v4 §3.5: runs.cancel is a
// proper Request method (not a notification). It returns 200 + JSON-RPC
// envelope with result, NOT 204 No Content.
func TestRunsCancelIsRequest(t *testing.T) {
	ts, api := newTestServer(t)
	defer ts.Close()

	// initialize first
	initBody := []byte(`{"jsonrpc":"2.0","id":1,"method":"runtime.initialize","params":{}}`)
	r1, err := netHTTP.Post(ts.URL+"/v1/rpc/runtime.initialize", "application/json", bytes.NewReader(initBody))
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	r1.Body.Close()

	body := []byte(`{"jsonrpc":"2.0","id":2,"method":"runs.cancel","params":{"runId":"r-123"}}`)
	resp, err := netHTTP.Post(ts.URL+"/v1/rpc/runs.cancel", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	defer resp.Body.Close()

	// Must be 200 + envelope, NOT 204.
	if resp.StatusCode != 200 {
		raw := readBody(resp)
		t.Fatalf("status = %d, want 200; body = %s", resp.StatusCode, raw)
	}
	var env struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  json.RawMessage `json:"result"`
		Error   *json.RawMessage `json:"error,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Error != nil {
		t.Fatalf("got error envelope: %s", string(*env.Error))
	}
	if string(env.ID) != "2" {
		t.Fatalf("id = %s, want 2", string(env.ID))
	}
	// Backend's CancelRun should have been invoked with the runId.
	if len(api.cancelledRuns) != 1 || api.cancelledRuns[0] != "r-123" {
		t.Fatalf("api.cancelledRuns = %v, want [r-123]", api.cancelledRuns)
	}
}

// readBody reads the response body into a string for diagnostic
// t.Fatalf messages. Best-effort — errors are swallowed because the
// caller is already in a failure path.
func readBody(r *netHTTP.Response) string {
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(r.Body)
	return buf.String()
}
