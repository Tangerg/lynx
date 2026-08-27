package http_test

import (
	"bytes"
	netHTTP "net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/scope/app/runtime/internal/delivery/operation"
	scopeapphttp "github.com/Tangerg/scope/app/runtime/internal/delivery/transport/http"
	"github.com/Tangerg/scope/app/runtime/protocol"
)

func TestDefaultCORSOriginsReturnsCallerOwnedConfiguration(t *testing.T) {
	origins := scopeapphttp.DefaultCORSOrigins()
	original := origins[0]
	origins[0] = "https://mutated.invalid"
	if scopeapphttp.DefaultCORSOrigins()[0] != original {
		t.Fatal("mutating one default origin list changed another caller's configuration")
	}
}

func TestDefaultCORSOriginsAuthorizeShippedDesktopClients(t *testing.T) {
	ts := newGatedServerWithOrigins(t, scopeapphttp.DefaultCORSOrigins())
	defer ts.Close()

	for _, origin := range []string{
		"wails://localhost",
		"http://wails.localhost",
		"http://127.0.0.1:5174",
		"http://localhost:5173",
	} {
		t.Run(origin, func(t *testing.T) {
			req, err := netHTTP.NewRequest(netHTTP.MethodOptions, ts.URL+"/v2/rpc", nil)
			if err != nil {
				t.Fatalf("new preflight: %v", err)
			}
			req.Header.Set("Origin", origin)
			req.Header.Set("Access-Control-Request-Method", netHTTP.MethodPost)
			req.Header.Set("Access-Control-Request-Headers", "Authorization, Content-Type")
			resp, err := netHTTP.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("preflight: %v", err)
			}
			defer resp.Body.Close()
			if got := resp.Header.Get("Access-Control-Allow-Origin"); got != origin {
				t.Fatalf("Allow-Origin = %q, want %q", got, origin)
			}
		})
	}

	if slices.Contains(scopeapphttp.DefaultCORSOrigins(), "*") {
		t.Fatal("default desktop CORS policy must not allow every browser origin")
	}
	for _, unrelated := range []string{"tauri://localhost", "http://localhost:3000"} {
		if slices.Contains(scopeapphttp.DefaultCORSOrigins(), unrelated) {
			t.Fatalf("default desktop CORS policy still allows unrelated origin %q", unrelated)
		}
	}
}

// newGatedServer builds a test server with the local-token gate +
// CORS allowlist set. Token is "test-token", origins is "http://app".
func newGatedServer(t *testing.T) *httptest.Server {
	return newGatedServerWithOrigins(t, []string{"http://app"})
}

func newGatedServerWithOrigins(t *testing.T, origins []string) *httptest.Server {
	t.Helper()
	srv, err := scopeapphttp.NewServer(scopeapphttp.Config{
		Endpoint:        newTestEndpoint(t, &fakeRuntime{}, operation.Config{}),
		Addr:            ":0",
		ServerInfo:      protocol.ServerInfo{Name: "scopeapp-test", Version: "0.0.0", InstanceID: testRuntimeInstanceID},
		ProtocolVersion: testProtocolVersion,
		LocalToken:      "test-token",
		CORSOrigins:     origins,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return httptest.NewServer(srv.Handler())
}

// TestAuthGateMissingToken — gate-on POST without Authorization gets
// 401 + a transport problem. Per API.md §7.3
// this MUST NOT use the JSON-RPC envelope.
func TestAuthGateMissingToken(t *testing.T) {
	ts := newGatedServer(t)
	defer ts.Close()

	body := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.discover"}`)
	resp, err := netHTTP.Post(ts.URL+"/v2/rpc", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != netHTTP.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	raw := readBody(resp)
	if !strings.Contains(raw, `"type":"urn:scopeapp:transport:unauthorized"`) {
		t.Fatalf("body = %s, want unauthorized problem", raw)
	}
	if strings.Contains(raw, `"jsonrpc"`) {
		t.Fatalf("401 must be flat JSON, got envelope: %s", raw)
	}
}

// TestAuthGate401HasChallenge — a 401 carries WWW-Authenticate: Bearer
// (RFC 9110 §15.5.2 / TRANSPORT §6.3 + §11).
func TestAuthGate401HasChallenge(t *testing.T) {
	ts := newGatedServer(t)
	defer ts.Close()

	body := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.discover"}`)
	resp, err := netHTTP.Post(ts.URL+"/v2/rpc", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != netHTTP.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	if got := resp.Header.Get("WWW-Authenticate"); !strings.Contains(got, "Bearer") {
		t.Fatalf("WWW-Authenticate = %q, must contain Bearer", got)
	}
}

// TestAuthGateWrongToken — wrong bearer also 401.
func TestAuthGateWrongToken(t *testing.T) {
	ts := newGatedServer(t)
	defer ts.Close()

	body := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.discover"}`)
	req, _ := netHTTP.NewRequest("POST", ts.URL+"/v2/rpc", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer wrong")
	req.Header.Set("Content-Type", "application/json")
	resp, err := netHTTP.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != netHTTP.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

// TestAuthGateCorrectToken — correct bearer goes through to the
// router. We only care that the token gate was cleared.
func TestAuthGateCorrectToken(t *testing.T) {
	ts := newGatedServer(t)
	defer ts.Close()

	// discover is an ordinary authenticated RPC method.
	body := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.discover","params":{}}`)
	req, _ := netHTTP.NewRequest("POST", ts.URL+"/v2/rpc", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	resp, err := netHTTP.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != netHTTP.StatusOK {
		raw := readBody(resp)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, raw)
	}
}

// TestAuthGateBypassesSidecars confirms operational endpoints stay open
// when the gate is on. Operations / oncall must always be able to
// curl these. TRANSPORT.md §安全.
func TestAuthGateBypassesSidecars(t *testing.T) {
	ts := newGatedServer(t)
	defer ts.Close()

	for _, path := range []string{"/v2/info", "/v2/health/live", "/v2/health/ready"} {
		resp, err := netHTTP.Get(ts.URL + path)
		if err != nil {
			t.Fatalf("get %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != netHTTP.StatusOK {
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

	req, _ := netHTTP.NewRequest("OPTIONS", ts.URL+"/v2/rpc", nil)
	req.Header.Set("Origin", "http://app")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Authorization, Content-Type")
	resp, err := netHTTP.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < netHTTP.StatusOK || resp.StatusCode >= 300 {
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

	body := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.discover","params":{}}`)
	req, _ := netHTTP.NewRequest("POST", ts.URL+"/v2/rpc", bytes.NewReader(body))
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

	body := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.discover","params":{}}`)
	req, _ := netHTTP.NewRequest("POST", ts.URL+"/v2/rpc", bytes.NewReader(body))
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
