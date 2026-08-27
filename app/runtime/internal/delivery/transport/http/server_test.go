package http_test

import (
	"bytes"
	"encoding/json"
	netHTTP "net/http"
	"strings"
	"testing"

	"github.com/Tangerg/scope/app/runtime/protocol"
)

// testProtocolVersion is what the test server is configured to announce. It reads
// the build's constant rather than a literal because the discover assertion below
// reaches the real ProtocolVersion — spelling the date here would make these
// transport tests fail on the next version bump for a reason that has nothing to do
// with transport.
const testProtocolVersion = protocol.ProtocolVersion

// TestSidecarInfo confirms /v2/info returns the minimal typed bootstrap shape.
func TestSidecarInfo(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	resp, err := netHTTP.Get(ts.URL + "/v2/info")
	if err != nil {
		t.Fatalf("get info: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != netHTTP.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body struct {
		ProtocolVersion string `json:"protocolVersion"`
		Server          struct {
			Name string `json:"name"`
		} `json:"server"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.ProtocolVersion != testProtocolVersion {
		t.Fatalf("protocolVersion = %q", body.ProtocolVersion)
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
	if resp.StatusCode != netHTTP.StatusOK {
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
	if resp.StatusCode != netHTTP.StatusOK {
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

// A binding must never accept metadata whose promise the operation does not
// implement. Embedded exposes method-specific option types; HTTP must refuse
// the same impossible combinations instead of silently discarding metadata.
func TestRPCRefusesMethodIncompatibleMetadata(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	tests := []struct {
		name   string
		body   string
		header string
		value  string
	}{
		{
			name:   "query idempotency key",
			body:   `{"jsonrpc":"2.0","id":"1","method":"runtime.discover","params":{}}`,
			header: "Idempotency-Key",
			value:  "query-must-not-promise-replay",
		},
		{
			name:   "namespace without key",
			body:   `{"jsonrpc":"2.0","id":"2","method":"runtime.discover","params":{}}`,
			header: "Idempotency-Namespace",
			value:  "idp_store",
		},
		{
			name:   "runtime subscription run cursor",
			body:   `{"jsonrpc":"2.0","id":"3","method":"runtime.subscribe","params":{"topics":["sessions.changed"]}}`,
			header: "Last-Event-Id",
			value:  "evt_cursor",
		},
		{
			name:   "run command cursor without replay key",
			body:   `{"jsonrpc":"2.0","id":"4","method":"runs.start","params":{"sessionId":"ses_1","input":[{"type":"text","text":"hi"}]}}`,
			header: "Last-Event-Id",
			value:  "evt_cursor",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req, err := netHTTP.NewRequest(
				netHTTP.MethodPost,
				ts.URL+"/v2/rpc",
				bytes.NewBufferString(test.body),
			)
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set(test.header, test.value)
			resp, err := netHTTP.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("post request: %v", err)
			}
			defer resp.Body.Close()

			if code := decodeErrorCode(t, resp); code != -32602 {
				t.Fatalf("error code = %d, want invalid_params (-32602)", code)
			}
		})
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
	if discoverResp.StatusCode != netHTTP.StatusOK {
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
	if unknownResp.StatusCode != netHTTP.StatusOK {
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
	if resp.StatusCode != netHTTP.StatusNotFound {
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
	if resp.StatusCode != netHTTP.StatusOK {
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
	if resp.StatusCode != netHTTP.StatusOK {
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

func TestIdempotencyNamespaceMismatchIsRefusedBeforeMutation(t *testing.T) {
	ts, api := newTestServer(t)
	defer ts.Close()

	req, err := netHTTP.NewRequest(
		netHTTP.MethodPost,
		ts.URL+"/v2/rpc",
		bytes.NewBufferString(`{"jsonrpc":"2.0","id":"1","method":"runs.cancel","params":{"runId":"run_1"}}`),
	)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "cancel-once")
	req.Header.Set("Idempotency-Namespace", "idp_replaced_store")
	resp, err := netHTTP.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post request: %v", err)
	}
	defer resp.Body.Close()
	if code := decodeErrorCode(t, resp); code != -32033 {
		t.Fatalf("mismatch code = %d, want -32033", code)
	}
	if len(api.canceledRuns) != 0 {
		t.Fatalf("mismatched request executed: canceled runs = %v", api.canceledRuns)
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
	if resp.StatusCode != netHTTP.StatusOK {
		raw := readBody(resp)
		t.Fatalf("status = %d, want 200; body = %s", resp.StatusCode, raw)
	}
}

// TestNonStringIDRejected confirms API.md §2.2: the envelope id must be a
// STRING. This is a transport-shape constraint, so a numeric id is rejected
// before dispatch instead of being coerced by the JSON-RPC SDK.
func TestNonStringIDRejected(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	body := []byte(`{"jsonrpc":"2.0","id":42,"method":"runtime.discover","params":{}}`)
	resp, err := netHTTP.Post(ts.URL+"/v2/rpc", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != netHTTP.StatusBadRequest {
		raw := readBody(resp)
		t.Fatalf("status = %d, want 400; body = %s", resp.StatusCode, raw)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/problem+json; charset=utf-8" {
		t.Fatalf("content-type = %q, want application/problem+json", got)
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

	if resp.StatusCode != netHTTP.StatusOK {
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
	var result struct {
		Type string          `json:"type"`
		Run  protocol.RunRef `json:"run"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil {
		t.Fatalf("decode cancel result %s: %v", env.Result, err)
	}
	if result.Type != "root" || result.Run.ID != "run_123" ||
		result.Run.Status != protocol.RunStatusFinished ||
		result.Run.Outcome == nil || result.Run.Outcome.Type != protocol.OutcomeCanceled {
		t.Fatalf("cancel result = %+v, want root run_123 finished/canceled", result)
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
	if resp.StatusCode != netHTTP.StatusNoContent {
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
	if resp.StatusCode != netHTTP.StatusRequestEntityTooLarge {
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
	if resp.StatusCode != netHTTP.StatusUnsupportedMediaType {
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

func TestInvalidRPCEnvelopeReturnsTransportProblem(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		detail string
	}{
		{
			name:   "duplicate member",
			body:   `{"jsonrpc":"2.0","id":"1","method":"sessions.list","method":"runs.list","params":{}}`,
			detail: `duplicate JSON member "method"`,
		},
		{
			name:   "empty method",
			body:   `{"jsonrpc":"2.0","id":"1","method":""}`,
			detail: "JSON-RPC request method is empty",
		},
		{
			name:   "explicit null id",
			body:   `{"jsonrpc":"2.0","id":null,"method":"runtime.discover","params":{}}`,
			detail: "JSON-RPC id must be a string; omit id for a notification",
		},
		{
			name:   "fractional numeric id",
			body:   `{"jsonrpc":"2.0","id":1.5,"method":"runtime.discover","params":{}}`,
			detail: "JSON-RPC id must be a string; omit id for a notification",
		},
		{
			name:   "out of range numeric id",
			body:   `{"jsonrpc":"2.0","id":1e100,"method":"runtime.discover","params":{}}`,
			detail: "JSON-RPC id must be a string; omit id for a notification",
		},
		{
			name:   "client response envelope",
			body:   `{"jsonrpc":"2.0","id":"1","result":{}}`,
			detail: "POST /v2/rpc accepts only JSON-RPC requests and notifications",
		},
		{
			name:   "request with response result",
			body:   `{"jsonrpc":"2.0","id":"1","method":"runtime.discover","params":{},"result":{}}`,
			detail: `unknown JSON-RPC request member "result"`,
		},
		{
			name:   "response with result and error",
			body:   `{"jsonrpc":"2.0","id":"1","result":{},"error":{"code":-1,"message":"bad"}}`,
			detail: "JSON-RPC response contains both result and error",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ts, _ := newTestServer(t)
			defer ts.Close()

			resp, err := netHTTP.Post(
				ts.URL+"/v2/rpc",
				"application/json",
				bytes.NewBufferString(test.body),
			)
			if err != nil {
				t.Fatalf("post: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != netHTTP.StatusBadRequest {
				t.Fatalf("status = %d, want 400", resp.StatusCode)
			}
			var problem struct {
				Type   string `json:"type"`
				Detail string `json:"detail"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&problem); err != nil {
				t.Fatalf("decode problem: %v", err)
			}
			if problem.Type != "urn:lyra:transport:invalid_request" ||
				!strings.Contains(problem.Detail, test.detail) {
				t.Fatalf("problem = %+v", problem)
			}
		})
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
	if resp.StatusCode != netHTTP.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
	if allow := resp.Header.Get("Allow"); !strings.Contains(allow, "POST") {
		t.Fatalf("Allow = %q, must list POST", allow)
	}
}
