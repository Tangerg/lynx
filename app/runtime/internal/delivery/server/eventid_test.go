package server

import (
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
	in <- runs.Event{RunID: "run_1", Seq: "00000000001", Timestamp: time.Unix(0, 0)}
	in <- runs.Event{RunID: "run_1", Seq: "00000000002", Timestamp: time.Unix(0, 0)}
	in <- runs.Event{RunID: "run_1", Seq: "00000000010", Timestamp: time.Unix(0, 0)}
	close(in)

	var ids []string
	for e := range mapRunEvents(in) {
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
