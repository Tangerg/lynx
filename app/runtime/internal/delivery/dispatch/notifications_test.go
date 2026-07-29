package dispatch

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
)

func TestEncodeRuntimeEventRejectsAnInvalidOutputShape(t *testing.T) {
	t.Parallel()

	_, err := EncodeRuntimeEvent(protocol.RuntimeEvent{
		Type: protocol.RuntimeResync, Sequence: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "RuntimeEvent.topics") {
		t.Fatalf("EncodeRuntimeEvent error = %v, want shape-qualified topics violation", err)
	}
}

func TestHandleNotificationSuppressesMetadataErrors(t *testing.T) {
	d := &Dispatcher{}
	msg := &transport.Request{
		Method: "client.unknown",
		Params: json.RawMessage(`{"_meta":null}`),
	}

	if got := d.Handle(context.Background(), msg); got.Response != nil {
		t.Fatalf("notification returned a response: %+v", got.Response)
	}
}

// TestStreamFilterOnlyDropsOptedOutEphemerals pins the one thing a client may
// suppress. An authoritative event is never filtered — not by an exclusion list
// naming it, and not by a client declaring nothing at all — because a client that
// cannot follow the run's stream is refused at negotiation, and one that IS serving
// the run has to receive every frame it must fold (§5.2 / §8.1).
func TestStreamFilterOnlyDropsOptedOutEphemerals(t *testing.T) {
	segmentStarted := protocol.StreamEvent{Type: protocol.StreamSegmentStarted}
	itemDelta := protocol.StreamEvent{Type: protocol.StreamItemDelta}

	if !streamFilterFrom(context.Background()).allow(segmentStarted) {
		t.Fatalf("missing capabilities should not filter events")
	}

	silent := protocol.ClientCapabilities{}
	ctx := protocol.WithRequestMeta(context.Background(), protocol.RequestMeta{ClientCapabilities: &silent})
	filter := streamFilterFrom(ctx)
	if !filter.allow(segmentStarted) || !filter.allow(itemDelta) {
		t.Fatalf("declaring no exclusions should suppress nothing")
	}

	opted := protocol.ClientCapabilities{
		ExcludedEphemeralEvents: []protocol.SuppressibleRunEventType{protocol.SuppressibleRunItemDelta},
	}
	ctx = protocol.WithRequestMeta(context.Background(), protocol.RequestMeta{ClientCapabilities: &opted})
	filter = streamFilterFrom(ctx)
	if !filter.allow(segmentStarted) {
		t.Fatalf("authoritative event should pass")
	}
	if filter.allow(itemDelta) {
		t.Fatalf("opted-out ephemeral event should be filtered")
	}
	if !filter.allow(protocol.StreamEvent{Type: protocol.StreamCustom}) {
		t.Fatalf("custom is always ephemeral but is not an opt-out event type")
	}
}

func TestCustomEventNeverCarriesAnSSEReplayID(t *testing.T) {
	t.Parallel()

	encode := runEventToFrameFor(context.Background())
	frame, ok := encode(protocol.RunEvent{
		RunID: "run_1", SegmentID: "seg_1", EventID: "evt_1",
		Event: protocol.StreamEvent{Type: protocol.StreamCustom, Name: "vendor.preview"},
	})
	if !ok {
		t.Fatal("custom event was not encoded")
	}
	if frame.SSEID != "" {
		t.Fatalf("custom SSE id = %q, want none", frame.SSEID)
	}
}
