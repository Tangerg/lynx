package operation

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func TestEventPolicyOnlyDropsOptedOutEphemerals(t *testing.T) {
	segmentStarted := protocol.RunEvent{Event: protocol.StreamEvent{Type: protocol.StreamSegmentStarted}}
	itemDelta := protocol.RunEvent{Event: protocol.StreamEvent{Type: protocol.StreamItemDelta}}

	if !allowsEvent(context.Background(), segmentStarted) {
		t.Fatal("missing capabilities should not filter events")
	}
	silent := protocol.ClientCapabilities{}
	ctx := WithRequestMeta(context.Background(), protocol.RequestMeta{ClientCapabilities: &silent})
	if !allowsEvent(ctx, segmentStarted) || !allowsEvent(ctx, itemDelta) {
		t.Fatal("declaring no exclusions should suppress nothing")
	}
	opted := protocol.ClientCapabilities{
		ExcludedEphemeralEvents: []protocol.SuppressibleRunEventType{protocol.SuppressibleRunItemDelta},
	}
	ctx = WithRequestMeta(context.Background(), protocol.RequestMeta{ClientCapabilities: &opted})
	if !allowsEvent(ctx, segmentStarted) {
		t.Fatal("authoritative event should pass")
	}
	if allowsEvent(ctx, itemDelta) {
		t.Fatal("opted-out ephemeral event should be filtered")
	}
}
