package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
)

// EncodeRunEvent wraps one RunEvent into a notifications.run.event
// JSON-RPC notification (API.md §5). The single downstream stream
// carries run / item / state events; run.finished (the terminal event)
// rides this same channel — there is no separate run-closed
// notification. runId + eventId let the client filter by stream and
// de-duplicate on reconnect (Last-Event-Id).
func EncodeRunEvent(ev protocol.RunEvent) (transport.Message, error) {
	return transport.NewNotification(NotificationRunEvent, ev)
}

// EncodeWorkspaceEvent wraps one WorkspaceEvent into a
// notifications.workspace.event notification (AUX_API §3.2). The single
// downstream channel carries every workspace event type (files/skills/mcp/
// resync) in the `event` field; clients branch on event.type. Ephemeral —
// no SSE id / replay (workspace state is "changed → re-fetch").
func EncodeWorkspaceEvent(ev protocol.WorkspaceEvent) (transport.Message, error) {
	return transport.NewNotification(NotificationWorkspaceEvent, struct {
		Event protocol.WorkspaceEvent `json:"event"`
	}{Event: ev})
}

// handleNotification dispatches the no-response methods. Errors are not
// surfaced over the wire (JSON-RPC notifications are fire-and-forget).
//
// notifications.canceled aborts an in-flight JSON-RPC request (matched
// by the carried envelope id); the transport layer owns request
// lifecycle and intercepts it upstream of Handle. We accept it here for
// protocol completeness.
func (d *Dispatcher) handleNotification(ctx context.Context, msg *transport.Request) {
	switch msg.Method {
	case MethodShutdown:
		var in protocol.ShutdownRequest
		_ = unmarshal(msg.Params, &in)
		_ = d.api.Shutdown(ctx, in)
	case NotificationCanceled:
		// no-op at this layer (see method doc)
	}
}

// runEventToFrame encodes a RunEvent into a notifications.run.event frame.
// Only durable events carry an SSE id: (TRANSPORT §9.3 / API §5.2) — replay
// must resume from a replayable event, never an ephemeral delta.
func runEventToFrame(ev protocol.RunEvent) (StreamFrame, bool) {
	notif, err := EncodeRunEvent(ev)
	if err != nil {
		return StreamFrame{}, false
	}
	sseID := ""
	if ev.Event.IsDurable() {
		sseID = ev.EventID
	}
	return StreamFrame{Notif: notif, SSEID: sseID}, true
}

// workspaceEventToFrame encodes a WorkspaceEvent into an ephemeral StreamFrame
// (no SSE id — workspace events aren't replayable).
func workspaceEventToFrame(ev protocol.WorkspaceEvent) (StreamFrame, bool) {
	notif, err := EncodeWorkspaceEvent(ev)
	if err != nil {
		return StreamFrame{}, false
	}
	return StreamFrame{Notif: notif}, true
}
