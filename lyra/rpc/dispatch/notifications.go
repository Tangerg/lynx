package dispatch

import (
	"encoding/json"
	"fmt"
	"time"

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
		Ts:      time.Now().UTC().Format(time.RFC3339Nano),
		Event:   json.RawMessage(body),
	})
}

// EncodeRunClosed produces a notifications/run/closed terminator
// (API.md §3.1 / §5.3 / §6.3). Carries the same runId clients used to
// filter notifications/run/event plus the terminal RunResult (stop
// reason + usage + cost) — read here, not by parsing the last event.
func EncodeRunClosed(runID string, result protocol.RunResult) (transport.Message, error) {
	return transport.NewNotification(NotificationRunClosed, struct {
		RunID  string             `json:"runId"`
		Result protocol.RunResult `json:"result"`
	}{RunID: runID, Result: result})
}

// EncodeRunClosedFrom reads the single terminal RunResult from results
// (defaulting to a completed result when results is nil or already
// closed) and encodes notifications/run/closed. Shared by every
// transport's stream pump so the drain-terminal-and-default logic lives
// in one place, not copied per transport.
func EncodeRunClosedFrom(runID string, results <-chan protocol.RunResult) (transport.Message, error) {
	result := protocol.RunResult{StopReason: "completed"}
	if results != nil {
		if r, ok := <-results; ok {
			result = r
		}
	}
	return EncodeRunClosed(runID, result)
}
