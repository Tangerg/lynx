package dispatch

import (
	"github.com/Tangerg/lynx/lyra/rpc/protocol"
	"github.com/Tangerg/lynx/lyra/rpc/transport"
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
