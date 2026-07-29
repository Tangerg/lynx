package dispatch

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
)

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
		ExcludedEphemeralEvents: []protocol.StreamEventType{protocol.StreamItemDelta},
	}
	ctx = protocol.WithRequestMeta(context.Background(), protocol.RequestMeta{ClientCapabilities: &opted})
	filter = streamFilterFrom(ctx)
	if !filter.allow(segmentStarted) {
		t.Fatalf("authoritative event should pass")
	}
	if filter.allow(itemDelta) {
		t.Fatalf("opted-out ephemeral event should be filtered")
	}
	durableCustom := protocol.StreamEvent{Type: protocol.StreamCustom, Durable: new(true)}
	optedCustom := protocol.ClientCapabilities{
		ExcludedEphemeralEvents: []protocol.StreamEventType{protocol.StreamCustom},
	}
	ctx = protocol.WithRequestMeta(context.Background(), protocol.RequestMeta{ClientCapabilities: &optedCustom})
	if !streamFilterFrom(ctx).allow(durableCustom) {
		t.Fatalf("a durable custom event is not suppressible by type")
	}
}
