package http_test

import (
	"bytes"
	"encoding/json"
	netHTTP "net/http"
	"strings"
	"testing"
)

const testProtocolVersion = "2026-07-19"

// TestSidecarInfo confirms /v2/info returns the minimal typed bootstrap shape.
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
		Protocol struct {
			Current string `json:"current"`
		} `json:"protocol"`
		Server struct {
			Name string `json:"name"`
		} `json:"server"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Protocol.Current != testProtocolVersion {
		t.Fatalf("protocol.current = %q", body.Protocol.Current)
	}
	if body.Server.Name != "lyra-test" {
		t.Fatalf("server.name = %q", body.Server.Name)
	}
}

// TestSidecarHealth confirms liveness is a dependency-free process check.
func TestSidecarHealth(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	resp, err := netHTTP.Get(ts.URL + "/v2/health/live")
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

// TestDiscoverOverRPC confirms POST /v2/rpc handles
// the request and wraps the result in a JSON-RPC envelope.
func TestDiscoverOverRPC(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	reqBody := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.discover","params":{}}`)
	resp, err := netHTTP.Post(ts.URL+"/v2/rpc", "application/json", bytes.NewReader(reqBody))
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

// TestRPCMethodHeader confirms X-Method reflects the envelope method.
func TestRPCMethodHeader(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	discoverBody := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.discover","params":{}}`)
	discoverResp, err := netHTTP.Post(ts.URL+"/v2/rpc", "application/json", bytes.NewReader(discoverBody))
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

	unknownBody := []byte(`{"jsonrpc":"2.0","id":"2","method":"test.unknown"}`)
	unknownResp, err := netHTTP.Post(ts.URL+"/v2/rpc", "application/json", bytes.NewReader(unknownBody))
	if err != nil {
		t.Fatalf("post unknown method: %v", err)
	}
	defer unknownResp.Body.Close()
	if unknownResp.StatusCode != 200 {
		t.Fatalf("unknown method status = %d", unknownResp.StatusCode)
	}
	if got := unknownResp.Header.Get("X-Method"); got != "test.unknown" {
		t.Fatalf("X-Method = %q, want test.unknown", got)
	}
}

// TestUnknownRPCEndpointReturns404 confirms 404 is reserved for HTTP routing.
func TestUnknownRPCEndpointReturns404(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	body := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.discover"}`)
	resp, err := netHTTP.Post(ts.URL+"/v2/rpc/runtime.discover", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		raw := readBody(resp)
		t.Fatalf("status = %d, want 404; body = %s", resp.StatusCode, raw)
	}
}

// TestUnknownMethodReturnsRPCError confirms method errors stay in a 200 envelope.
func TestUnknownMethodReturnsRPCError(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	discoverBody := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.discover","params":{}}`)
	r1, _ := netHTTP.Post(ts.URL+"/v2/rpc", "application/json", bytes.NewReader(discoverBody))
	r1.Body.Close()

	body := []byte(`{"jsonrpc":"2.0","id":"2","method":"runs.unknownMethod"}`)
	resp, err := netHTTP.Post(ts.URL+"/v2/rpc", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
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
	resp, err := netHTTP.Post(ts.URL+"/v2/rpc", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		raw := readBody(resp)
		t.Fatalf("status = %d, want 200; body = %s", resp.StatusCode, raw)
	}
	if len(api.canceledRuns) != 1 || api.canceledRuns[0] != "run_before_discover" {
		t.Fatalf("api.canceledRuns = %v, want [run_before_discover]", api.canceledRuns)
	}
}

func TestIdempotencyKeyReplaysMutationAndRejectsReuse(t *testing.T) {
	ts, api := newTestServer(t)
	defer ts.Close()

	post := func(body string) *netHTTP.Response {
		t.Helper()
		req, err := netHTTP.NewRequest(netHTTP.MethodPost, ts.URL+"/v2/rpc", bytes.NewBufferString(body))
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "cancel-once")
		resp, err := netHTTP.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("post request: %v", err)
		}
		return resp
	}

	first := post(`{"jsonrpc":"2.0","id":"1","method":"runs.cancel","params":{"runId":"run_1"}}`)
	first.Body.Close()
	replay := post(`{"jsonrpc":"2.0","id":"2","method":"runs.cancel","params":{"runId":"run_1"}}`)
	replay.Body.Close()
	if len(api.canceledRuns) != 1 || api.canceledRuns[0] != "run_1" {
		t.Fatalf("canceled runs = %v, want one run_1", api.canceledRuns)
	}

	conflict := post(`{"jsonrpc":"2.0","id":"3","method":"runs.cancel","params":{"runId":"run_2"}}`)
	defer conflict.Body.Close()
	if code := decodeErrorCode(t, conflict); code != -32020 {
		t.Fatalf("conflict code = %d, want -32020", code)
	}
	if len(api.canceledRuns) != 1 {
		t.Fatalf("conflicting request executed: canceled runs = %v", api.canceledRuns)
	}
}

// TestRPCUsesEnvelopeMethod confirms the endpoint needs no URL method segment.
func TestRPCUsesEnvelopeMethod(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	body := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.discover","params":{}}`)
	resp, err := netHTTP.Post(ts.URL+"/v2/rpc", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		raw := readBody(resp)
		t.Fatalf("status = %d, want 200; body = %s", resp.StatusCode, raw)
	}
}

// TestNonStringIDRejected confirms API.md §2.2: the envelope id must be
// a STRING; a numeric id is rejected with invalid_request (-32600).
func TestNonStringIDRejected(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	body := []byte(`{"jsonrpc":"2.0","id":42,"method":"runtime.discover","params":{}}`)
	resp, err := netHTTP.Post(ts.URL+"/v2/rpc", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		raw := readBody(resp)
		t.Fatalf("status = %d, want 200; body = %s", resp.StatusCode, raw)
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
	r1, err := netHTTP.Post(ts.URL+"/v2/rpc", "application/json", bytes.NewReader(discoverBody))
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	r1.Body.Close()

	body := []byte(`{"jsonrpc":"2.0","id":"2","method":"runs.cancel","params":{"runId":"run_123"}}`)
	resp, err := netHTTP.Post(ts.URL+"/v2/rpc", "application/json", bytes.NewReader(body))
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
	if len(api.canceledRuns) != 1 || api.canceledRuns[0] != "run_123" {
		t.Fatalf("api.canceledRuns = %v, want [run_123]", api.canceledRuns)
	}
}

// TestNotificationReturns204 confirms a client→server notification (no
// envelope id) is acknowledged with 204 No Content and no response body
// (TRANSPORT §6.3 picks 204 over 202 — dispatch is already complete).
func TestNotificationReturns204(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	discoverBody := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.discover","params":{}}`)
	r1, _ := netHTTP.Post(ts.URL+"/v2/rpc", "application/json", bytes.NewReader(discoverBody))
	r1.Body.Close()

	// test.notification has no id ⇒ it's a Notification; JSON-RPC
	// never sends a response for one, so the transport acks with 204.
	body := []byte(`{"jsonrpc":"2.0","method":"test.notification","params":{"id":"1"}}`)
	resp, err := netHTTP.Post(ts.URL+"/v2/rpc", "application/json", bytes.NewReader(body))
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
	resp, err := netHTTP.Post(ts.URL+"/v2/rpc", "application/json", bytes.NewReader(big))
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
	resp, err := netHTTP.Post(ts.URL+"/v2/rpc", "text/plain", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 415 {
		t.Fatalf("status = %d, want 415", resp.StatusCode)
	}
}

func TestMalformedRPCBodyReturnsTransportProblem(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	resp, err := netHTTP.Post(ts.URL+"/v2/rpc", "application/json", bytes.NewBufferString(`{"jsonrpc":`))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != netHTTP.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/problem+json; charset=utf-8" {
		t.Fatalf("content-type = %q, want application/problem+json", got)
	}
	var problem struct {
		Type      string `json:"type"`
		RequestID string `json:"requestId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem.Type != "urn:lyra:transport:invalid_request" || problem.RequestID == "" {
		t.Fatalf("problem = %+v", problem)
	}
}

// TestMethodNotAllowedHasAllow confirms a wrong HTTP method on a known
// endpoint returns 405 with an Allow header listing the supported methods
// (RFC 9110 §15.5.6 / TRANSPORT §6.3). chi populates Allow from the route.
func TestMethodNotAllowedHasAllow(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	// /v2/rpc is POST-only; a GET to it is 405, not 404.
	req, _ := netHTTP.NewRequest("GET", ts.URL+"/v2/rpc", nil)
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
