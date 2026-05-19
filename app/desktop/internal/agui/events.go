// Package agui implements a minimal AG-UI protocol server.
//
// AG-UI (Agent-User Interaction) is an event-oriented protocol — the server
// streams events over an HTTP SSE response and the client folds them into
// view state. Spec: https://docs.ag-ui.com
//
// Only the event types this mock needs are constructed here; the wire shape
// matches the @ag-ui/core schemas exactly so the JS client validates cleanly.
package agui

import "encoding/json"

// EventType — string values must match @ag-ui/core's EventType enum.
type EventType string

const (
	EventRunStarted              EventType = "RUN_STARTED"
	EventRunFinished             EventType = "RUN_FINISHED"
	EventRunError                EventType = "RUN_ERROR"
	EventStepStarted             EventType = "STEP_STARTED"
	EventStepFinished            EventType = "STEP_FINISHED"
	EventTextMessageStart        EventType = "TEXT_MESSAGE_START"
	EventTextMessageContent      EventType = "TEXT_MESSAGE_CONTENT"
	EventTextMessageEnd          EventType = "TEXT_MESSAGE_END"
	EventToolCallStart           EventType = "TOOL_CALL_START"
	EventToolCallArgs            EventType = "TOOL_CALL_ARGS"
	EventToolCallEnd             EventType = "TOOL_CALL_END"
	EventToolCallResult          EventType = "TOOL_CALL_RESULT"
	EventReasoningStart          EventType = "REASONING_START"
	EventReasoningMessageStart   EventType = "REASONING_MESSAGE_START"
	EventReasoningMessageContent EventType = "REASONING_MESSAGE_CONTENT"
	EventReasoningMessageEnd     EventType = "REASONING_MESSAGE_END"
	EventReasoningEnd            EventType = "REASONING_END"
	EventCustom                  EventType = "CUSTOM"
)

// Event is the wire shape: a flat map serialized as JSON. AG-UI events all
// carry `type` plus event-specific fields, so a generic map is simpler than
// a hand-written union and still round-trips identically.
type Event map[string]any

// Constructors — one per event we emit. Kept close to the schemas in
// @ag-ui/core/dist/index.d.ts.

func RunStarted(threadID, runID string) Event {
	return Event{"type": EventRunStarted, "threadId": threadID, "runId": runID}
}

func RunFinished(threadID, runID string) Event {
	return Event{"type": EventRunFinished, "threadId": threadID, "runId": runID}
}

func StepStarted(name string) Event {
	return Event{"type": EventStepStarted, "stepName": name}
}

func TextMessageStart(messageID, role string) Event {
	return Event{"type": EventTextMessageStart, "messageId": messageID, "role": role}
}

func TextMessageContent(messageID, delta string) Event {
	return Event{"type": EventTextMessageContent, "messageId": messageID, "delta": delta}
}

func TextMessageEnd(messageID string) Event {
	return Event{"type": EventTextMessageEnd, "messageId": messageID}
}

func ToolCallStart(toolCallID, toolCallName, parentMessageID string) Event {
	e := Event{
		"type":         EventToolCallStart,
		"toolCallId":   toolCallID,
		"toolCallName": toolCallName,
	}
	if parentMessageID != "" {
		e["parentMessageId"] = parentMessageID
	}
	return e
}

func ToolCallArgs(toolCallID, delta string) Event {
	return Event{"type": EventToolCallArgs, "toolCallId": toolCallID, "delta": delta}
}

// ToolCallEndExtras carries non-standard summary fields that ride along
// TOOL_CALL_END for the demo (added/removed/hits/lines/durationMs). AG-UI's
// schema uses passthrough — extra fields survive validation.
type ToolCallEndExtras struct {
	Status     string // "ok" | "err"
	DurationMs int
	Added      *int
	Removed    *int
	Hits       *int
	Lines      *int
}

func ToolCallEnd(toolCallID string, x ToolCallEndExtras) Event {
	e := Event{"type": EventToolCallEnd, "toolCallId": toolCallID}
	if x.Status != "" {
		e["status"] = x.Status
	}
	if x.DurationMs > 0 {
		e["durationMs"] = x.DurationMs
	}
	if x.Added != nil {
		e["added"] = *x.Added
	}
	if x.Removed != nil {
		e["removed"] = *x.Removed
	}
	if x.Hits != nil {
		e["hits"] = *x.Hits
	}
	if x.Lines != nil {
		e["lines"] = *x.Lines
	}
	return e
}

func Custom(name string, value any) Event {
	return Event{"type": EventCustom, "name": name, "value": value}
}

// ReasoningStart opens a "thinking" span. Optional — the reasoning *message*
// pair is what carries the actual text.
func ReasoningStart(messageID string) Event {
	return Event{"type": EventReasoningStart, "messageId": messageID}
}

// ReasoningMessageStart begins a reasoning text stream. `parentMessageId` is a
// passthrough extension we use to attach reasoning to a parent assistant
// message — AG-UI's schema allows extra fields on event objects.
func ReasoningMessageStart(messageID, parentMessageID string) Event {
	e := Event{
		"type":      EventReasoningMessageStart,
		"messageId": messageID,
		"role":      "reasoning",
	}
	if parentMessageID != "" {
		e["parentMessageId"] = parentMessageID
	}
	return e
}

func ReasoningMessageContent(messageID, delta string) Event {
	return Event{"type": EventReasoningMessageContent, "messageId": messageID, "delta": delta}
}

func ReasoningMessageEnd(messageID string) Event {
	return Event{"type": EventReasoningMessageEnd, "messageId": messageID}
}

func ReasoningEnd(messageID string) Event {
	return Event{"type": EventReasoningEnd, "messageId": messageID}
}

// Marshal returns the JSON form used as the SSE `data:` payload.
func (e Event) Marshal() ([]byte, error) {
	return json.Marshal(e)
}

// IntPtr is a convenience for the optional summary fields.
func IntPtr(v int) *int { return &v }
