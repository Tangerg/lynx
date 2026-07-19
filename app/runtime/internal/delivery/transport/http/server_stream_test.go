package http_test

import (
	"bytes"
	"context"
	netHTTP "net/http"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	lyratransport "github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
)

// StartRun lets the fake drive the streamable path: it returns a runId ack plus
// a pre-baked, pre-closed RunEvent channel so a POST runs.start exercises
// serveStream end-to-end.
func (f *fakeRuntime) StartRun(_ context.Context, in protocol.StartRunRequest) (*protocol.StartRunResponse, <-chan protocol.RunEvent, error) {
	ch := make(chan protocol.RunEvent, 2)
	ch <- protocol.RunEvent{RunID: "run_x", EventID: "evt_00000000001",
		Event: protocol.StreamEvent{Type: protocol.StreamSegmentStarted, Run: &protocol.RunRef{ID: "run_x", SessionID: in.SessionID}}}
	ch <- protocol.RunEvent{RunID: "run_x", EventID: "evt_00000000002",
		Event: protocol.StreamEvent{Type: protocol.StreamSegmentFinished, Outcome: &protocol.RunOutcome{Type: protocol.OutcomeCompleted, Result: &protocol.RunResult{}}}}
	close(ch)
	return &protocol.StartRunResponse{RunID: "run_x"}, ch, nil
}

type sseFrame struct{ id, data string }

// parseSSE splits a text/event-stream body into frames, lifting the id and
// data lines and skipping comments / blanks.
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

// TestStreamableRunStart confirms a streaming method's POST response is itself
// the event stream: 200 text/event-stream, first frame is the JSON-RPC ack, then
// run-event frames each carrying eventId as the SSE id.
func TestStreamableRunStart(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	discoverBody := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.discover","params":{}}`)
	r0, _ := netHTTP.Post(ts.URL+"/v2/rpc", "application/json", bytes.NewReader(discoverBody))
	r0.Body.Close()

	body := []byte(`{"jsonrpc":"2.0","id":"2","method":"runs.start","params":{"sessionId":"ses_1","input":[{"type":"text","text":"hi"}]}}`)
	req, _ := netHTTP.NewRequest("POST", ts.URL+"/v2/rpc", bytes.NewReader(body))
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
	if frames[1].id != "evt_00000000001" || !strings.Contains(frames[1].data, "segment.started") {
		t.Fatalf("frame[1] = %+v, want segment.started @ evt 1", frames[1])
	}
	if frames[2].id != "evt_00000000002" || !strings.Contains(frames[2].data, "segment.finished") {
		t.Fatalf("frame[2] = %+v, want segment.finished @ evt 2", frames[2])
	}
}

// SubscribeRun records the reconnect cursor so the test below can assert the
// transport plumbed the Last-Event-Id header onto the dispatch ctx.
func (f *fakeRuntime) SubscribeRun(ctx context.Context, runID string) (*protocol.StartRunResponse, <-chan protocol.RunEvent, error) {
	f.gotLastEventID = lyratransport.LastEventIDFrom(ctx)
	ch := make(chan protocol.RunEvent)
	close(ch)
	return &protocol.StartRunResponse{RunID: runID}, ch, nil
}

// TestSubscribeCarriesLastEventID confirms the transport lifts the
// Last-Event-Id request header onto the ctx so runs.subscribe resumes from it
// instead of full-replaying.
func TestSubscribeCarriesLastEventID(t *testing.T) {
	ts, api := newTestServer(t)
	defer ts.Close()

	discoverBody := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.discover","params":{}}`)
	r0, _ := netHTTP.Post(ts.URL+"/v2/rpc", "application/json", bytes.NewReader(discoverBody))
	r0.Body.Close()

	body := []byte(`{"jsonrpc":"2.0","id":"2","method":"runs.subscribe","params":{"runId":"run_1"}}`)
	req, _ := netHTTP.NewRequest("POST", ts.URL+"/v2/rpc", bytes.NewReader(body))
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
