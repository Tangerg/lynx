package http_test

import (
	"bytes"
	"context"
	"iter"
	netHTTP "net/http"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// What one streaming connection occupies (Batch D4, the half that does not need a
// running desktop).
//
// Every streamable response starts a bridge goroutine: the frame source blocks
// between events and cannot be selected on, so one goroutine ranges it and feeds
// a channel the write loop waits on alongside the heartbeat and the client's
// disconnect. Whether that goroutine goes away when the client does was argued in
// a comment — the request context unwinds the source, so the range ends — and
// never demonstrated. A leak there is invisible in every functional test and
// costs one goroutine per connection for the life of the process, which on a
// desktop client that reconnects on every network blip is the whole process.
//
// The assertion is deliberately about SHAPE, not about a count: residue must not
// scale with the number of connections. An exact goroutine number would be
// hostage to net/http's own pooling and would teach whoever hits it to raise the
// bound rather than find the leak.

// blockingRuntime streams one event and then waits, which is what a run looks
// like nearly all the time: an event, then a long wait on the model. A finite
// sequence would let the bridge exit on its own and prove nothing about
// disconnect.
type blockingRuntime struct {
	released chan struct{}
}

func (b *blockingRuntime) Discover(context.Context) (*protocol.DiscoverResponse, error) {
	return &protocol.DiscoverResponse{Protocol: protocol.SupportedProtocolRange()}, nil
}

func (b *blockingRuntime) StartRun(ctx context.Context, in protocol.StartRunRequest) (*protocol.StartRunResponse, iter.Seq[protocol.RunEvent], error) {
	events := func(yield func(protocol.RunEvent) bool) {
		// Signal on the way out, whichever way that is. A disconnect can unwind this
		// source through its context OR by making yield report false (the bridge
		// abandoned the range while a frame was in flight) — both are the source
		// letting go, and a test that only recognized one of them would call the
		// other a leak.
		defer func() {
			select {
			case b.released <- struct{}{}:
			default:
			}
		}()
		// One event, then park: the test disconnects while this goroutine is here,
		// which is the state a leak hides in.
		if !yield(protocol.RunEvent{
			RunID: "run_block", EventID: "evt_00000000001",
			Event: protocol.StreamEvent{
				Type: protocol.StreamSegmentStarted,
				Run: &protocol.RunRef{
					RunSummary:      protocol.RunSummary{ID: "run_block", SessionID: in.SessionID},
					ActiveSegmentID: "seg_block",
				},
			},
		}) {
			return
		}
		<-ctx.Done()
	}
	return &protocol.StartRunResponse{
		RunID: "run_block", SegmentID: "seg_block", UserItemID: "item_block",
	}, events, nil
}

// settledGoroutines polls until the count stops falling, so a residue reading is
// taken after the runtime's own teardown has run rather than in the middle of it.
func settledGoroutines() int {
	last := runtime.NumGoroutine()
	for range 40 {
		time.Sleep(25 * time.Millisecond)
		now := runtime.NumGoroutine()
		if now >= last {
			return now
		}
		last = now
	}
	return last
}

func TestStreamingConnectionsReleaseTheirGoroutines(t *testing.T) {
	const connections = 16

	api := &blockingRuntime{released: make(chan struct{}, connections)}
	ts := newTestServerFor(t, api)
	defer ts.Close()

	// One warm-up connection first: the server's per-connection machinery and the
	// client's transport are lazily created, and counting them as a leak would make
	// the test fail for the one reason it is not looking for.
	openAndAbandon(t, ts.URL, api)
	ts.Client().CloseIdleConnections()
	baseline := settledGoroutines()

	for range connections {
		openAndAbandon(t, ts.URL, api)
	}
	ts.Client().CloseIdleConnections()
	residue := settledGoroutines() - baseline

	// A per-connection leak would show as ~`connections`. Anything below half of
	// them cannot be per-connection, and this is not the place to police the rest:
	// net/http keeps its own goroutines around on its own schedule.
	if residue >= connections/2 {
		t.Fatalf("after %d streams opened and dropped, %d goroutines remain above a baseline of %d — "+
			"a residue that scales with connections is a leak per connection",
			connections, residue, baseline)
	}
	t.Logf("%d streams left %d goroutines above a baseline of %d", connections, residue, baseline)
}

// openAndAbandon opens one streaming POST, reads far enough to know the server is
// inside serveStream with a live source, then drops the connection the way a
// client that loses its network does — without a graceful end-of-stream.
func openAndAbandon(t *testing.T, url string, api *blockingRuntime) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	body := bytes.NewReader([]byte(`{"jsonrpc":"2.0","id":"1","method":"runs.start",` +
		`"params":{"sessionId":"ses_1","input":[{"type":"text","text":"hi"}]}}`))
	req, err := netHTTP.NewRequestWithContext(ctx, netHTTP.MethodPost, url+"/v2/rpc", body)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := netHTTP.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST runs.start: %v", err)
	}

	// Assert the precondition instead of inferring it: a request the server refuses
	// still returns a readable body, and this test would then wait out its timeout
	// and report a leak that never happened.
	if resp.StatusCode != netHTTP.StatusOK {
		t.Fatalf("status = %d, want 200 — the stream never opened", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream — this response is not a stream", ct)
	}

	// Read one chunk so the handler has written the ack and is in its select loop.
	buf := make([]byte, 1)
	if _, err := resp.Body.Read(buf); err != nil {
		t.Fatalf("read first byte: %v", err)
	}
	cancel()
	_ = resp.Body.Close()

	// The source observing its context is the mechanism the bridge relies on to
	// unwind. Waiting for it here is what separates "the goroutine left" from "the
	// goroutine has not noticed yet".
	select {
	case <-api.released:
	case <-time.After(2 * time.Second):
		t.Fatal("the frame source never observed the client disconnect")
	}
}
