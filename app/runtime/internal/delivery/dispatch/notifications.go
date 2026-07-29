package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
)

// EncodeRunEvent wraps one RunEvent into a notifications.run.event
// JSON-RPC notification (API.md §5). The single downstream stream
// carries run / item / state events; segment.finished (the terminal event)
// rides this same stream — there is no separate run-closed
// notification. runId + eventId let the client filter by stream and
// de-duplicate on reconnect (Last-Event-Id).
func EncodeRunEvent(ev protocol.RunEvent) (transport.Message, error) {
	return transport.NewNotification(NotificationRunEvent, ev)
}

// EncodeRuntimeEvent wraps one RuntimeEvent into a notifications.runtime.event
// notification (§7.3). The single downstream stream carries every topic's change
// signal in the `event` field; clients branch on event.type. Ephemeral by design — no
// SSE id, no replay: every frame is "this moved, read it again", and a client that
// missed one refetches rather than replays.
func EncodeRuntimeEvent(ev protocol.RuntimeEvent) (transport.Message, error) {
	return transport.NewNotification(NotificationRuntimeEvent, struct {
		Event protocol.RuntimeEvent `json:"event"`
	}{Event: ev})
}

// Client notifications currently have no public methods. Unknown
// notifications are intentionally ignored as required by JSON-RPC.
func (d *Dispatcher) handleNotification(context.Context, *transport.Request) {}

// runEventToFrameFor returns the per-request encoder for RunEvent stream
// notifications. Every authoritative event goes out; a client's opt-out only
// suppresses ephemeral previews, so final state stays recoverable from the stream
// alone (§5.2).
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
		if ev.Event.Replayable() {
			sseID = ev.EventID
		}
		return StreamFrame{Notif: notif, SSEID: sseID}, true
	}
}

// runtimeEventToFrame encodes a RuntimeEvent into an ephemeral StreamFrame (no SSE
// id — a change signal is not replayable, and does not need to be).
func runtimeEventToFrame(ev protocol.RuntimeEvent) (StreamFrame, bool) {
	notif, err := EncodeRuntimeEvent(ev)
	if err != nil {
		return StreamFrame{}, false
	}
	return StreamFrame{Notif: notif}, true
}

// streamFilter is the client's ephemeral opt-out, and nothing else. There is
// deliberately no "types the client declared" set to intersect with: a client that
// cannot follow the authoritative stream must be refused the run (§8.1 capability
// negotiation), not handed a shortened stream it would mistake for the whole one.
type streamFilter struct {
	optOut map[protocol.SuppressibleRunEventType]bool
}

func streamFilterFrom(ctx context.Context) streamFilter {
	caps, ok := protocol.ClientCapabilitiesFrom(ctx)
	if !ok {
		return streamFilter{}
	}
	return streamFilter{optOut: eventSet(caps.ExcludedEphemeralEvents)}
}

func eventSet(events []protocol.SuppressibleRunEventType) map[protocol.SuppressibleRunEventType]bool {
	if events == nil {
		return nil
	}
	set := make(map[protocol.SuppressibleRunEventType]bool, len(events))
	for _, ev := range events {
		set[ev] = true
	}
	return set
}

func (f streamFilter) allow(ev protocol.StreamEvent) bool {
	return f.optOut == nil || !f.optOut[protocol.SuppressibleRunEventType(ev.Type)]
}
