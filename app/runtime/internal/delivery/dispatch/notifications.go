package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
)

// EncodeRunEvent wraps one RunEvent into a notifications.run.event
// JSON-RPC notification (API.md §5). The single downstream stream
// carries run / item / state events; segment.finished (the terminal event)
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

// runEventToFrameFor returns the per-request encoder for RunEvent stream
// notifications. Client capabilities gate which event types this stream may
// receive; opt-out only suppresses ephemeral events so final state remains
// recoverable.
func runEventToFrameFor(ctx context.Context) func(protocol.RunEvent) (StreamFrame, bool) {
	filter := streamFilterFrom(ctx)
	return func(ev protocol.RunEvent) (StreamFrame, bool) {
		if !filter.allow(ev.Event) {
			return StreamFrame{}, false
		}
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

type streamFilter struct {
	declared map[protocol.StreamEventType]bool
	optOut   map[protocol.StreamEventType]bool
}

func streamFilterFrom(ctx context.Context) streamFilter {
	caps, ok := protocol.ClientCapabilitiesFrom(ctx)
	if !ok {
		return streamFilter{}
	}
	return streamFilter{
		declared: eventSet(caps.Events),
		optOut:   eventSet(caps.OptOutNotificationMethods),
	}
}

func eventSet(events []protocol.StreamEventType) map[protocol.StreamEventType]bool {
	if events == nil {
		return nil
	}
	set := make(map[protocol.StreamEventType]bool, len(events))
	for _, ev := range events {
		set[ev] = true
	}
	return set
}

func (f streamFilter) allow(ev protocol.StreamEvent) bool {
	if f.declared != nil && !f.declared[ev.Type] {
		return false
	}
	if !ev.IsDurable() && f.optOut != nil && f.optOut[ev.Type] {
		return false
	}
	return true
}
