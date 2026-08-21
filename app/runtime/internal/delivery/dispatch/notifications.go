package dispatch

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

const (
	NotificationRunEvent     = "notifications.run.event"
	NotificationRuntimeEvent = "notifications.runtime.event"
)

// EncodeRunEvent wraps one RunEvent into a notifications.run.event
// JSON-RPC notification (API.md §5). The single downstream stream
// carries segment / item / Plan events; segment.finished (the terminal event)
// rides this same stream — there is no separate run-closed
// notification. runId + eventId let the client filter by stream and
// de-duplicate on reconnect (Last-Event-Id).
func EncodeRunEvent(event protocol.RunEvent) (transport.Message, error) {
	if err := protocol.ValidateWireTree(event); err != nil {
		return nil, fmt.Errorf("encode run event: %w", err)
	}
	return transport.NewNotification(NotificationRunEvent, event)
}

// EncodeRuntimeEvent wraps one RuntimeEvent into a notifications.runtime.event
// notification (§7.3). The single downstream stream carries every topic's change
// signal in the `event` field; clients branch on event.type. Ephemeral by design — no
// SSE id, no replay: every frame is "this moved, read it again", and a client that
// missed one refetches rather than replays.
func EncodeRuntimeEvent(event protocol.RuntimeEvent) (transport.Message, error) {
	if err := protocol.ValidateWireTree(event); err != nil {
		return nil, fmt.Errorf("encode runtime event: %w", err)
	}
	return transport.NewNotification(NotificationRuntimeEvent, protocol.RuntimeEventNotification{
		Event: event,
	})
}

// Client notifications currently have no public methods. Unknown
// notifications are intentionally ignored as required by JSON-RPC.
func (r *Router) handleNotification(context.Context, *transport.Request) {}

// runEventToFrameFor returns the per-request encoder for RunEvent stream
// notifications. Every authoritative event goes out; a client's opt-out only
// suppresses ephemeral previews, so every final fact stays recoverable from the stream
// alone (§5.2).
func runEventToFrame(event protocol.RunEvent) (StreamFrame, bool) {
	notification, err := EncodeRunEvent(event)
	if err != nil {
		return StreamFrame{}, false
	}
	sseID := ""
	if event.Event.Replayable() {
		sseID = event.EventID
	}
	return StreamFrame{Notification: notification, SSEID: sseID}, true
}

// runtimeEventToFrame encodes a RuntimeEvent into an ephemeral StreamFrame (no SSE
// id — a change signal is not replayable, and does not need to be).
func runtimeEventToFrame(event protocol.RuntimeEvent) (StreamFrame, bool) {
	notification, err := EncodeRuntimeEvent(event)
	if err != nil {
		return StreamFrame{}, false
	}
	return StreamFrame{Notification: notification}, true
}

func frameOperationEvent(event any) (StreamFrame, bool) {
	switch typed := event.(type) {
	case protocol.RunEvent:
		return runEventToFrame(typed)
	case protocol.RuntimeEvent:
		return runtimeEventToFrame(typed)
	default:
		return StreamFrame{}, false
	}
}
