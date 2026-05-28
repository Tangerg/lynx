package rpcadapter

import (
	"encoding/json"
	"fmt"

	"github.com/Tangerg/lynx/lyra/pkg/coreapi"
	"github.com/Tangerg/lynx/lyra/pkg/transport"
)

// EncodeRunEvent wraps an AG-UI event into a notifications/run/event
// JSON-RPC Notification (API.md §3.1). runID + eventId let the client
// filter by stream + de-duplicate on reconnect via Last-Event-Id.
//
// API.md v4 §3.1 cut: the older `streamHandle` field is gone — the
// runId IS the stream identifier.
func EncodeRunEvent(runID, eventID string, ev coreapi.AgUiEvent) (*transport.Message, error) {
	if ev == nil {
		return nil, fmt.Errorf("rpcadapter: nil ag-ui event")
	}
	body, err := ev.ToJSON()
	if err != nil {
		return nil, fmt.Errorf("rpcadapter: encode ag-ui event: %w", err)
	}
	params, err := json.Marshal(coreapi.RunEvent{
		RunID:   runID,
		EventID: eventID,
		Event:   body,
	})
	if err != nil {
		return nil, fmt.Errorf("rpcadapter: marshal run event params: %w", err)
	}
	return &transport.Message{
		JSONRPC: transport.JSONRPCVersion,
		Method:  NotificationRunEvent,
		Params:  params,
	}, nil
}

// EncodeRunClosed produces a notifications/run/closed terminator
// (API.md §3.1). Carries the same runId clients used to filter
// notifications/run/event.
func EncodeRunClosed(runID, reason string) (*transport.Message, error) {
	params, err := json.Marshal(struct {
		RunID  string `json:"runId"`
		Reason string `json:"reason,omitempty"`
	}{RunID: runID, Reason: reason})
	if err != nil {
		return nil, fmt.Errorf("rpcadapter: marshal run closed params: %w", err)
	}
	return &transport.Message{
		JSONRPC: transport.JSONRPCVersion,
		Method:  NotificationRunClosed,
		Params:  params,
	}, nil
}
