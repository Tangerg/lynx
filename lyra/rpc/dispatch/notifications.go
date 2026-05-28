package dispatch

import (
	"encoding/json"
	"fmt"

	"github.com/Tangerg/lynx/lyra/rpc/protocol"
	"github.com/Tangerg/lynx/lyra/rpc/transport"
)

// EncodeRunEvent wraps an AG-UI event into a notifications/run/event
// JSON-RPC Notification (API.md §3.1). runID + eventId let the client
// filter by stream + de-duplicate on reconnect via Last-Event-Id.
//
// API.md v4 §3.1 cut: the older `streamHandle` field is gone — the
// runId IS the stream identifier.
func EncodeRunEvent(runID, eventID string, ev protocol.AgUiEvent) (transport.Message, error) {
	if ev == nil {
		return nil, fmt.Errorf("dispatch: nil ag-ui event")
	}
	body, err := ev.ToJSON()
	if err != nil {
		return nil, fmt.Errorf("dispatch: encode ag-ui event: %w", err)
	}
	return transport.NewNotification(NotificationRunEvent, protocol.RunEvent{
		RunID:   runID,
		EventID: eventID,
		Event:   json.RawMessage(body),
	})
}

// EncodeRunClosed produces a notifications/run/closed terminator
// (API.md §3.1). Carries the same runId clients used to filter
// notifications/run/event.
func EncodeRunClosed(runID, reason string) (transport.Message, error) {
	return transport.NewNotification(NotificationRunClosed, struct {
		RunID  string `json:"runId"`
		Reason string `json:"reason,omitempty"`
	}{RunID: runID, Reason: reason})
}
