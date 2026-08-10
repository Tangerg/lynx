package dispatch

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
	"github.com/Tangerg/lynx/app/runtime/protocol"
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

func TestDispatchNotificationSuppressesMetadataErrors(t *testing.T) {
	router := &Router{}
	message := &transport.Request{
		Method: "client.unknown",
		Params: json.RawMessage(`{"_meta":null}`),
	}

	if got := router.Dispatch(context.Background(), message); got.Response != nil {
		t.Fatalf("notification returned a response: %+v", got.Response)
	}
}

func TestCustomEventNeverCarriesAnSSEReplayID(t *testing.T) {
	t.Parallel()

	frame, ok := runEventToFrame(protocol.RunEvent{
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
