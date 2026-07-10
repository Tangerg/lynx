package http_test

import (
	"bytes"
	"encoding/json"
	netHTTP "net/http"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

const testProtocolVersion = "2026-06-07"

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

// TestDiscoverOverRPC confirms POST /v2/rpc/runtime.discover handles
// the request and wraps the result in a JSON-RPC envelope.
func TestDiscoverOverRPC(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	reqBody := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.discover","params":{}}`)
	resp, err := netHTTP.Post(ts.URL+"/v2/rpc/runtime.discover", "application/json", bytes.NewReader(reqBody))
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

	discoverBody := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.discover","params":{}}`)
	discoverResp, err := netHTTP.Post(ts.URL+"/v2/rpc/runtime.discover", "application/json", bytes.NewReader(discoverBody))
	if err != nil {
		t.Fatalf("post discover: %v", err)
	}
	discoverResp.Body.Close()
	if discoverResp.StatusCode != 200 {
		t.Fatalf("discover status = %d", discoverResp.StatusCode)
	}
	if got := discoverResp.Header.Get("X-Method"); got != "runtime.discover" {
		t.Fatalf("X-Method = %q, want runtime.discover", got)
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
	resp, err := netHTTP.Post(ts.URL+"/v2/rpc/runtime.discover", "application/json", bytes.NewReader(body))
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

	discoverBody := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.discover","params":{}}`)
	r1, _ := netHTTP.Post(ts.URL+"/v2/rpc/runtime.discover", "application/json", bytes.NewReader(discoverBody))
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

// TestBusinessMethodDoesNotRequireDiscover confirms runtime.discover is only a
// stateless information query; business methods do not require it first.
func TestBusinessMethodDoesNotRequireDiscover(t *testing.T) {
	ts, api := newTestServer(t)
	defer ts.Close()

	body := []byte(`{"jsonrpc":"2.0","id":"1","method":"runs.cancel","params":{"runId":"run_before_discover"}}`)
	resp, err := netHTTP.Post(ts.URL+"/v2/rpc/runs.cancel", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		raw := readBody(resp)
		t.Fatalf("status = %d, want 200; body = %s", resp.StatusCode, raw)
	}
	if len(api.cancelledRuns) != 1 || api.cancelledRuns[0] != "run_before_discover" {
		t.Fatalf("api.cancelledRuns = %v, want [run_before_discover]", api.cancelledRuns)
	}
}

// TestRPCWithoutMethodReturns404 confirms POST to /v2/rpc without a
// {method} suffix is unrouted — 404, no JSON-RPC envelope.
func TestRPCWithoutMethodReturns404(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	body := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.discover","params":{}}`)
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

	body := []byte(`{"jsonrpc":"2.0","id":42,"method":"runtime.discover","params":{}}`)
	resp, err := netHTTP.Post(ts.URL+"/v2/rpc/runtime.discover", "application/json", bytes.NewReader(body))
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

	discoverBody := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.discover","params":{}}`)
	r1, err := netHTTP.Post(ts.URL+"/v2/rpc/runtime.discover", "application/json", bytes.NewReader(discoverBody))
	if err != nil {
		t.Fatalf("discover: %v", err)
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

	discoverBody := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.discover","params":{}}`)
	r1, _ := netHTTP.Post(ts.URL+"/v2/rpc/runtime.discover", "application/json", bytes.NewReader(discoverBody))
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
	resp, err := netHTTP.Post(ts.URL+"/v2/rpc/runtime.discover", "application/json", bytes.NewReader(big))
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

	body := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.discover","params":{}}`)
	resp, err := netHTTP.Post(ts.URL+"/v2/rpc/runtime.discover", "text/plain", bytes.NewReader(body))
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
