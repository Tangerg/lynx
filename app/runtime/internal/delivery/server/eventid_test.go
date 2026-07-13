package server

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
)

// TestMapRunEvents_FramesWireEventID verifies delivery applies the evt_ wire
// framing to the application's opaque cursor (§11.2): the Coordinator mints a
// prefix-free, fixed-width, monotonic Seq, and mapRunEvents presents it as
// evt_<cursor> on the wire. The fixed width makes lexical comparison agree with
// numeric, which the SSE replay path relies on.
func TestMapRunEvents_FramesWireEventID(t *testing.T) {
	in := make(chan runs.Event, 3)
	in <- runs.Event{RunID: "run_1", Seq: "00000000001", Timestamp: time.Unix(0, 0), Payload: runs.SegmentProgressed{}}
	in <- runs.Event{RunID: "run_1", Seq: "00000000002", Timestamp: time.Unix(0, 0), Payload: runs.SegmentProgressed{}}
	in <- runs.Event{RunID: "run_1", Seq: "00000000010", Timestamp: time.Unix(0, 0), Payload: runs.SegmentProgressed{}}
	close(in)

	var ids []string
	for e := range mapRunEvents(context.Background(), in) {
		if !strings.HasPrefix(e.EventID, "evt_") {
			t.Fatalf("eventId %q missing evt_ prefix", e.EventID)
		}
		ids = append(ids, e.EventID)
	}

	want := []string{"evt_00000000001", "evt_00000000002", "evt_00000000010"}
	if len(ids) != len(want) {
		t.Fatalf("got %d events, want %d", len(ids), len(want))
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("eventId[%d] = %q, want %q", i, ids[i], want[i])
		}
		if i > 0 && ids[i] <= ids[i-1] { // fixed-width padding → lexical == numeric order
			t.Fatalf("eventIds not strictly increasing: %q then %q", ids[i-1], ids[i])
		}
	}
}

// TestMapRunEvents_ExitsOnClientDisconnect proves the mapper goroutine doesn't
// leak when the client disconnects while an event is in flight: with nobody
// draining the wire channel the mapper blocks on send, and only the request ctx
// can free it — the source channel closing can't unblock a stuck send. Detected
// via goroutine count, because *reading* the wire channel would itself unblock
// the send and hide the leak.
func TestMapRunEvents_ExitsOnClientDisconnect(t *testing.T) {
	before := runtime.NumGoroutine()
	in := make(chan runs.Event) // unbuffered: the send below rendezvous with the mapper
	ctx, cancel := context.WithCancel(context.Background())
	_ = mapRunEvents(ctx, in)

	// Hand the mapper one event; it reads it, then blocks on the wire send because
	// no one drains the returned channel — the leak condition.
	in <- runs.Event{RunID: "run_1", Seq: "00000000001", Timestamp: time.Unix(0, 0), Payload: runs.SegmentProgressed{}}
	cancel() // client disconnect must free the blocked mapper

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= before {
			return // mapper exited — no leak
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("mapRunEvents goroutine did not exit after ctx cancel — leaked")
}
