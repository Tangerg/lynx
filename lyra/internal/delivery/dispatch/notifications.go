package dispatch

import (
	"github.com/Tangerg/lynx/lyra/internal/delivery/protocol"
	"github.com/Tangerg/lynx/lyra/internal/delivery/transport"
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

// workspaceEventToFrame encodes a WorkspaceEvent into an ephemeral StreamFrame
// (no SSE id — workspace events aren't replayable).
func workspaceEventToFrame(ev protocol.WorkspaceEvent) (StreamFrame, bool) {
	notif, err := EncodeWorkspaceEvent(ev)
	if err != nil {
		return StreamFrame{}, false
	}
	return StreamFrame{Notif: notif}, true
}
