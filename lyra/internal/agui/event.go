// Package agui implements the AG-UI protocol's event types and a
// translator from Lyra's internal [chat.Event] stream to the
// AG-UI shape.
//
// Reference: https://docs.ag-ui.com/concepts/events
//
// Scope is deliberately narrow: this package only knows about the
// AG-UI event vocabulary + how to map Lyra's events into it.
// Transport (HTTP+SSE / WebSocket / IPC) is somebody else's
// concern — [SSEEncoder] is provided as a convenience for the
// common SSE-over-HTTP case but the package itself doesn't host a
// server.
package agui

import "time"

// Event is the sealed interface every AG-UI event implements. The
// JSON tag on each concrete type sets the `type` discriminator
// per the protocol spec.
//
// Implementations are value types — the translator owns the
// channel they flow through, so copying on assignment is the
// right idiom (no aliasing surprises across translator state).
type Event interface {
	// EventType returns the protocol-level type string (e.g.
	// "RunStarted"). The struct already encodes the same value
	// in its JSON `type` field; this accessor is for type
	// switches that don't want to round-trip through JSON.
	EventType() string
}

// ------------------------------------------------------------------
// Run lifecycle
// ------------------------------------------------------------------

// RunStarted opens the AG-UI conversation thread for one Lyra turn.
// ThreadID maps to Lyra's SessionID; RunID to Lyra's TurnID.
type RunStarted struct {
	Type        string    `json:"type"`
	Timestamp   time.Time `json:"timestamp,omitzero"`
	ThreadID    string    `json:"threadId"`
	RunID       string    `json:"runId"`
	ParentRunID string    `json:"parentRunId,omitempty"`
}

func (RunStarted) EventType() string { return "RunStarted" }

// RunFinished closes a successful run.
type RunFinished struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp,omitzero"`
}

func (RunFinished) EventType() string { return "RunFinished" }

// RunError closes a failed run with a human-readable message + an
// optional stable code (e.g. "ENGINE_ERROR" / "PLANNING_ERROR" —
// the same codes Lyra emits on its [chat.ErrorEvent]).
type RunError struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp,omitzero"`
	Message   string    `json:"message"`
	Code      string    `json:"code,omitempty"`
}

func (RunError) EventType() string { return "RunError" }

// ------------------------------------------------------------------
// Text message lifecycle
// ------------------------------------------------------------------

// TextMessageStart opens a new streaming assistant message. The
// translator emits this lazily — on the first [chat.MessageDelta]
// after a turn starts, or after a tool round closed the previous
// message.
type TextMessageStart struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp,omitzero"`
	MessageID string    `json:"messageId"`
	Role      string    `json:"role"` // "assistant" for Lyra
}

func (TextMessageStart) EventType() string { return "TextMessageStart" }

// TextMessageContent carries one chunk of streamed text. Multiple
// of these arrive between TextMessageStart and TextMessageEnd.
type TextMessageContent struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp,omitzero"`
	MessageID string    `json:"messageId"`
	Delta     string    `json:"delta"`
}

func (TextMessageContent) EventType() string { return "TextMessageContent" }

// TextMessageEnd closes a streaming message. The translator emits
// it before opening a tool round or at TurnEnd.
type TextMessageEnd struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp,omitzero"`
	MessageID string    `json:"messageId"`
}

func (TextMessageEnd) EventType() string { return "TextMessageEnd" }

// ------------------------------------------------------------------
// Tool call lifecycle
// ------------------------------------------------------------------

// ToolCallStart announces a tool invocation. ToolCallID is the
// opaque correlator that ties Start / Args / End / Result together.
type ToolCallStart struct {
	Type            string    `json:"type"`
	Timestamp       time.Time `json:"timestamp,omitzero"`
	ToolCallID      string    `json:"toolCallId"`
	ToolCallName    string    `json:"toolCallName"`
	ParentMessageID string    `json:"parentMessageId,omitempty"`
}

func (ToolCallStart) EventType() string { return "ToolCallStart" }

// ToolCallArgs carries the call's arguments. Lyra knows the full
// args upfront (the model already emitted them) so the translator
// sends a single ToolCallArgs with the complete JSON; the
// streaming form (multiple Args chunks) is still on-protocol but
// unused here.
type ToolCallArgs struct {
	Type       string    `json:"type"`
	Timestamp  time.Time `json:"timestamp,omitzero"`
	ToolCallID string    `json:"toolCallId"`
	Delta      string    `json:"delta"`
}

func (ToolCallArgs) EventType() string { return "ToolCallArgs" }

// ToolCallEnd closes the args portion of a tool call. The result
// arrives separately via [ToolCallResult].
type ToolCallEnd struct {
	Type       string    `json:"type"`
	Timestamp  time.Time `json:"timestamp,omitzero"`
	ToolCallID string    `json:"toolCallId"`
}

func (ToolCallEnd) EventType() string { return "ToolCallEnd" }

// ToolCallResult carries the tool's output, correlated to the
// originating call by ToolCallID. MessageID is set to the active
// run id so the AG-UI client renders the result under the right
// thread.
type ToolCallResult struct {
	Type       string    `json:"type"`
	Timestamp  time.Time `json:"timestamp,omitzero"`
	MessageID  string    `json:"messageId"`
	ToolCallID string    `json:"toolCallId"`
	Content    string    `json:"content"`
	Role       string    `json:"role,omitempty"`
}

func (ToolCallResult) EventType() string { return "ToolCallResult" }

// ------------------------------------------------------------------
// Custom — for events outside the standard vocabulary
// ------------------------------------------------------------------

// Custom is the AG-UI escape hatch for vendor-specific events.
// Lyra uses it for [chat.PlanGenerated] — there is no
// first-class "plan proposed" event in AG-UI v1.
type Custom struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp,omitzero"`
	Name      string    `json:"name"`
	Value     any       `json:"value"`
}

func (Custom) EventType() string { return "Custom" }
