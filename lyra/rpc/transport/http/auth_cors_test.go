package http_test

import (
	"bytes"
	netHTTP "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/lyra/rpc/protocol"
	lyrahttp "github.com/Tangerg/lynx/lyra/rpc/transport/http"
)

// newGatedServer builds a test server with the local-token gate +
// CORS allowlist set. Token is "test-token", origins is "http://app".
func newGatedServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv, err := lyrahttp.NewServer(lyrahttp.Config{
		Runtime:         &fakeRuntime{},
		Addr:            ":0",
		ServerInfo:      protocol.ServerInfo{Name: "lyra-test", Version: "0.0.0"},
		ProtocolVersion: testProtocolVersion,
		Capabilities:    protocol.ServerCapabilities{},
		LocalToken:      "test-token",
		CORSOrigins:     []string{"http://app"},
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return httptest.NewServer(srv.Handler())
}

// TestAuthGateMissingToken — gate-on POST without Authorization gets
// 401 + flat-JSON `{"error":"missing_local_token"}`. Per API.md §7.3
// this MUST NOT use the JSON-RPC envelope.
func TestAuthGateMissingToken(t *testing.T) {
	ts := newGatedServer(t)
	defer ts.Close()

	body := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.ping"}`)
	resp, err := netHTTP.Post(ts.URL+"/v2/rpc/runtime.ping", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	raw := readBody(resp)
	if !strings.Contains(raw, `"missing_local_token"`) {
		t.Fatalf("body = %s, want missing_local_token", raw)
	}
	if strings.Contains(raw, `"jsonrpc"`) {
		t.Fatalf("401 must be flat JSON, got envelope: %s", raw)
	}
}

// TestAuthGateEchoesTraceID — 401 body carries `traceId` echoed
// from the request's X-Trace-Id header (FE
// RpcTransportError.traceId contract).
func TestAuthGateEchoesTraceID(t *testing.T) {
	ts := newGatedServer(t)
	defer ts.Close()

	body := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.ping"}`)
	req, _ := netHTTP.NewRequest("POST", ts.URL+"/v2/rpc/runtime.ping", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Trace-Id", "trace-42")
	resp, err := netHTTP.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	raw := readBody(resp)
	if !strings.Contains(raw, `"traceId":"trace-42"`) {
		t.Fatalf("body = %s, must echo traceId", raw)
	}
}

// TestAuthGateWrongToken — wrong bearer also 401.
func TestAuthGateWrongToken(t *testing.T) {
	ts := newGatedServer(t)
	defer ts.Close()

	body := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.ping"}`)
	req, _ := netHTTP.NewRequest("POST", ts.URL+"/v2/rpc/runtime.ping", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer wrong")
	req.Header.Set("Content-Type", "application/json")
	resp, err := netHTTP.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

// TestAuthGateCorrectToken — correct bearer goes through to the
// dispatcher (which, since we haven't initialize-d, returns protocol
// violation — but we only care that we cleared the gate).
func TestAuthGateCorrectToken(t *testing.T) {
	ts := newGatedServer(t)
	defer ts.Close()

	// initialize is allowed pre-handshake
	body := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.initialize","params":{}}`)
	req, _ := netHTTP.NewRequest("POST", ts.URL+"/v2/rpc/runtime.initialize", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	resp, err := netHTTP.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		raw := readBody(resp)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, raw)
	}
}

// TestAuthGateBypassesSidecars — /v2/info and /v2/health stay open
// when the gate is on. Operations / oncall must always be able to
// curl these. TRANSPORT.md §安全.
func TestAuthGateBypassesSidecars(t *testing.T) {
	ts := newGatedServer(t)
	defer ts.Close()

	for _, path := range []string{"/v2/info", "/v2/health"} {
		resp, err := netHTTP.Get(ts.URL + path)
		if err != nil {
			t.Fatalf("get %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("%s status = %d, want 200", path, resp.StatusCode)
		}
	}
}

// TestCORSPreflight — OPTIONS request from allowed origin returns a 2xx
// + Allow-Origin echoes + Allow-Headers includes Authorization. Gate
// stays out of the way because cors resolves preflight before authGate.
// (go-chi/cors answers 200; the contract is silent on the exact 2xx.)
func TestCORSPreflight(t *testing.T) {
	ts := newGatedServer(t)
	defer ts.Close()

	req, _ := netHTTP.NewRequest("OPTIONS", ts.URL+"/v2/rpc/runtime.initialize", nil)
	req.Header.Set("Origin", "http://app")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Authorization, Content-Type")
	resp, err := netHTTP.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("status = %d, want 2xx", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "http://app" {
		t.Fatalf("Allow-Origin = %q, want http://app", got)
	}
	if got := resp.Header.Get("Access-Control-Allow-Headers"); !strings.Contains(got, "Authorization") {
		t.Fatalf("Allow-Headers = %q, must include Authorization", got)
	}
	if got := resp.Header.Get("Access-Control-Allow-Methods"); !strings.Contains(got, "POST") {
		t.Fatalf("Allow-Methods = %q, must include POST", got)
	}
}

// TestCORSAllowedOriginOnPost — actual POST from an allowed origin
// echoes Allow-Origin + Vary: Origin + Allow-Credentials.
func TestCORSAllowedOriginOnPost(t *testing.T) {
	ts := newGatedServer(t)
	defer ts.Close()

	body := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.initialize","params":{}}`)
	req, _ := netHTTP.NewRequest("POST", ts.URL+"/v2/rpc/runtime.initialize", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://app")
	resp, err := netHTTP.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "http://app" {
		t.Fatalf("Allow-Origin = %q, want http://app", got)
	}
	if got := resp.Header.Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("Allow-Credentials = %q, want true", got)
	}
	if got := resp.Header.Get("Vary"); !strings.Contains(got, "Origin") {
		t.Fatalf("Vary = %q, must include Origin", got)
	}
	if got := resp.Header.Get("Access-Control-Expose-Headers"); !strings.Contains(got, "X-Method") {
		t.Fatalf("Expose-Headers = %q, must include X-Method", got)
	}
}

// TestCORSDisallowedOrigin — request from a non-allowlisted origin
// gets no Allow-Origin header (the browser will reject the response).
// We don't 4xx in this case — the request itself is well-formed.
func TestCORSDisallowedOrigin(t *testing.T) {
	ts := newGatedServer(t)
	defer ts.Close()

	body := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.initialize","params":{}}`)
	req, _ := netHTTP.NewRequest("POST", ts.URL+"/v2/rpc/runtime.initialize", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://evil")
	resp, err := netHTTP.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Allow-Origin = %q, want empty for disallowed origin", got)
	}
}

// TestIssueLocalToken — IssueLocalToken creates the file at the
// requested path with mode 0600 and returns a non-empty token.
func TestIssueLocalToken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "token")

	tok, err := lyrahttp.IssueLocalToken(path)
	if err != nil {
		t.Fatalf("IssueLocalToken: %v", err)
	}
	if tok.Value == "" {
		t.Fatalf("empty token value")
	}
	if tok.Path != path {
		t.Fatalf("path = %q, want %q", tok.Path, path)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("mode = %o, want 600", mode)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != tok.Value {
		t.Fatalf("file content mismatch")
	}
}
