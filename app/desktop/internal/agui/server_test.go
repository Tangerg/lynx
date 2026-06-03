package agui

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
)

// post issues a JSON-RPC request and returns the decoded response.
func post(t *testing.T, base, connID, method, body string) rpcResponse {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, base+"/v2/rpc/"+method, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if connID != "" {
		req.Header.Set("X-Conn-Id", connID)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s: %v", method, err)
	}
	defer res.Body.Close()
	var out rpcResponse
	_ = json.NewDecoder(res.Body).Decode(&out)
	return out
}

func TestV2MockHandshakeSessionAndRun(t *testing.T) {
	srv := httptest.NewServer(New("").handler())
	defer srv.Close()

	// Sidecar info exposes the v2 protocol version.
	res, err := http.Get(srv.URL + "/v2/info")
	if err != nil {
		t.Fatal(err)
	}
	var info map[string]any
	_ = json.NewDecoder(res.Body).Decode(&info)
	res.Body.Close()
	if info["protocolVersion"] != ProtocolVersion {
		t.Fatalf("info protocolVersion = %v", info["protocolVersion"])
	}

	// initialize + create session.
	if r := post(t, srv.URL, "c1", "runtime.initialize",
		`{"jsonrpc":"2.0","id":"1","method":"runtime.initialize","params":{}}`); r.Result == nil {
		t.Fatal("initialize returned no result")
	}
	create := post(t, srv.URL, "c1", "sessions.create", `{"jsonrpc":"2.0","id":"2","method":"sessions.create","params":{"title":"t"}}`)
	sid, _ := create.Result.(map[string]any)["id"].(string)
	if !strings.HasPrefix(sid, "ses_") {
		t.Fatalf("session id = %q", sid)
	}

	// Open the SSE stream, then start a run; collect frames until run.finished.
	streamReq, _ := http.NewRequest(http.MethodGet, srv.URL+"/v2/rpc/stream?conn=c1", nil)
	streamRes, err := http.DefaultClient.Do(streamReq)
	if err != nil {
		t.Fatal(err)
	}
	defer streamRes.Body.Close()

	start := post(t, srv.URL, "c1",
		"runs.start", `{"jsonrpc":"2.0","id":"3","method":"runs.start","params":{"sessionId":"`+sid+`","input":[{"type":"text","text":"hi"}]}}`)
	if _, ok := start.Result.(map[string]any)["runId"]; !ok {
		t.Fatal("runs.start returned no runId")
	}

	types := collectEventTypes(t, streamRes, "run.finished")
	for _, want := range []string{"run.started", "item.started", "item.delta", "item.completed", "run.finished"} {
		if !slices.Contains(types, want) {
			t.Fatalf("stream missing %q; got %v", want, types)
		}
	}
}

func TestV2MockApprovalInterrupt(t *testing.T) {
	srv := httptest.NewServer(New("").handler())
	defer srv.Close()

	streamReq, _ := http.NewRequest(http.MethodGet, srv.URL+"/v2/rpc/stream?conn=c2", nil)
	streamRes, err := http.DefaultClient.Do(streamReq)
	if err != nil {
		t.Fatal(err)
	}
	defer streamRes.Body.Close()

	post(t, srv.URL, "c2", "runs.start",
		`{"jsonrpc":"2.0","id":"9","method":"runs.start","params":{"sessionId":"ses_x","input":[{"type":"text","text":"please rm the build"}]}}`)

	// The dangerous input ends the run with an approval interrupt.
	frames := collectFrames(t, streamRes, "run.finished")
	last := frames[len(frames)-1]
	outcome := last["event"].(map[string]any)["outcome"].(map[string]any)
	if outcome["type"] != "interrupt" {
		t.Fatalf("expected interrupt outcome, got %v", outcome["type"])
	}
}

// TestV2MockTransportStatusCodes locks the transport-layer status code
// contract (TRANSPORT.md §6.3): notifications ack with 204, a wrong HTTP
// method returns 405 + Allow, and a url/body method mismatch is a 400
// (not 409).
func TestV2MockTransportStatusCodes(t *testing.T) {
	srv := httptest.NewServer(New("").handler())
	defer srv.Close()

	// Notification (no id) → 204, no body.
	notif, _ := http.NewRequest(http.MethodPost, srv.URL+"/v2/rpc/runtime.shutdown",
		strings.NewReader(`{"jsonrpc":"2.0","method":"runtime.shutdown","params":{}}`))
	notif.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(notif)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("notification status = %d, want 204", res.StatusCode)
	}

	// Wrong HTTP method → 405 + Allow (RFC 9110 §15.5.6).
	get, _ := http.NewRequest(http.MethodGet, srv.URL+"/v2/rpc/runs.start", nil)
	res, err = http.DefaultClient.Do(get)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET status = %d, want 405", res.StatusCode)
	}
	if allow := res.Header.Get("Allow"); !strings.Contains(allow, "POST") {
		t.Fatalf("405 Allow = %q, want it to list POST", allow)
	}

	// URL method ≠ body method → 400 (self-inconsistent request, not 409).
	bad, _ := http.NewRequest(http.MethodPost, srv.URL+"/v2/rpc/runs.start",
		strings.NewReader(`{"jsonrpc":"2.0","id":"1","method":"runs.cancel","params":{}}`))
	bad.Header.Set("Content-Type", "application/json")
	res, err = http.DefaultClient.Do(bad)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("method-mismatch status = %d, want 400", res.StatusCode)
	}
}

// collectFrames reads RunEvent payloads from an SSE body until one whose
// event.type == stopType arrives (or a timeout).
func collectFrames(t *testing.T, res *http.Response, stopType string) []map[string]any {
	t.Helper()
	type result struct {
		frames []map[string]any
	}
	done := make(chan result, 1)
	go func() {
		var frames []map[string]any
		sc := bufio.NewScanner(res.Body)
		for sc.Scan() {
			line := sc.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var note struct {
				Params map[string]any `json:"params"`
			}
			if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &note) != nil {
				continue
			}
			frames = append(frames, note.Params)
			if ev, ok := note.Params["event"].(map[string]any); ok && ev["type"] == stopType {
				done <- result{frames}
				return
			}
		}
		_ = sc.Err()
		done <- result{frames}
	}()
	select {
	case r := <-done:
		return r.frames
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for stream")
		return nil
	}
}

func collectEventTypes(t *testing.T, res *http.Response, stopType string) []string {
	frames := collectFrames(t, res, stopType)
	types := make([]string, 0, len(frames))
	for _, f := range frames {
		if ev, ok := f["event"].(map[string]any); ok {
			if ty, ok := ev["type"].(string); ok {
				types = append(types, ty)
			}
		}
	}
	return types
}
