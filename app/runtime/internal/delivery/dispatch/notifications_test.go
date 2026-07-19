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

func TestStreamFilterEventDeclarations(t *testing.T) {
	runStarted := protocol.StreamEvent{Type: protocol.StreamSegmentStarted}
	itemDelta := protocol.StreamEvent{Type: protocol.StreamItemDelta}

	if !streamFilterFrom(context.Background()).allow(runStarted) {
		t.Fatalf("missing capabilities should not filter events")
	}

	emptyDeclared := protocol.ClientCapabilities{Events: []protocol.StreamEventType{}}
	ctx := protocol.WithRequestMeta(context.Background(), protocol.RequestMeta{ClientCapabilities: &emptyDeclared})
	if streamFilterFrom(ctx).allow(runStarted) {
		t.Fatalf("explicit empty events should filter all events")
	}

	declared := protocol.ClientCapabilities{
		Events:         []protocol.StreamEventType{protocol.StreamSegmentStarted, protocol.StreamItemDelta},
		ExcludedEvents: []protocol.StreamEventType{protocol.StreamItemDelta},
	}
	ctx = protocol.WithRequestMeta(context.Background(), protocol.RequestMeta{ClientCapabilities: &declared})
	filter := streamFilterFrom(ctx)
	if !filter.allow(runStarted) {
		t.Fatalf("declared durable event should pass")
	}
	if filter.allow(itemDelta) {
		t.Fatalf("opted-out ephemeral event should be filtered")
	}
}
