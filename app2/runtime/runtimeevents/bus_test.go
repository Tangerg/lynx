package runtimeevents

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

func TestSubscribeStartsWithWireValidColdResync(t *testing.T) {
	bus, err := New(Config{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(bus.Close)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	_, events, err := bus.Subscribe(ctx, protocol.RuntimeSubscribeRequest{
		Topics: []protocol.RuntimeTopic{protocol.TopicSessionsChanged},
	})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	for event := range events {
		if err := protocol.ValidateWireTree(event); err != nil {
			t.Fatalf("cold event is not wire-valid: %+v: %v", event, err)
		}
		if event.Type != protocol.RuntimeResync || event.Sequence != 1 {
			t.Fatalf("cold event = %+v", event)
		}
		return
	}
	t.Fatal("subscription closed before its cold resync")
}
