package server

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/scope/app/runtime/internal/application/runs"
)

// TestMapRunEvents_FramesWireEventID verifies delivery applies the evt_ wire
// framing to the stream position the application minted (§11.2), and nothing
// else: the cursor's contents are the application's business, and this layer
// neither parses nor orders them.
func TestMapRunEvents_FramesWireEventID(t *testing.T) {
	cursors := []string{"AAAA", "BBBB", "CCCC"}
	in := slices.Values([]runs.Event{
		{RunID: "run_1", Cursor: cursors[0], Timestamp: time.Unix(0, 0), Payload: runs.SegmentProgressed{}},
		{RunID: "run_1", Cursor: cursors[1], Timestamp: time.Unix(0, 0), Payload: runs.SegmentProgressed{}},
		{RunID: "run_1", Cursor: cursors[2], Timestamp: time.Unix(0, 0), Payload: runs.SegmentProgressed{}},
	})

	var ids []string
	for e := range mapRunEvents(context.Background(), in) {
		if !strings.HasPrefix(e.EventID, "evt_") {
			t.Fatalf("eventId %q missing evt_ prefix", e.EventID)
		}
		ids = append(ids, e.EventID)
	}

	if len(ids) != len(cursors) {
		t.Fatalf("got %d events, want %d", len(ids), len(cursors))
	}
	for i, cursor := range cursors {
		if want := "evt_" + cursor; ids[i] != want {
			t.Fatalf("eventId[%d] = %q, want %q", i, ids[i], want)
		}
	}
}

func TestMapRunEvents_ContainsPresenterPanic(t *testing.T) {
	in := slices.Values([]runs.Event{
		{RunID: "run_1", Cursor: "AAAA"}, // nil payload is invalid
		{RunID: "run_1", Cursor: "BBBB", Payload: runs.SegmentProgressed{}},
	})

	var count int
	for range mapRunEvents(context.Background(), in) {
		count++
	}
	if count != 0 {
		t.Fatalf("events after presenter panic = %d, want 0", count)
	}
}

func TestMapRunEvents_DoesNotRecoverConsumerPanic(t *testing.T) {
	in := slices.Values([]runs.Event{
		{RunID: "run_1", Cursor: "AAAA", Payload: runs.SegmentProgressed{}},
	})
	const want = "consumer panic"

	defer func() {
		if got := recover(); got != want {
			t.Fatalf("recovered panic = %v, want %q", got, want)
		}
	}()
	for range mapRunEvents(context.Background(), in) {
		panic(want)
	}
	t.Fatal("consumer panic was recovered by mapRunEvents")
}
