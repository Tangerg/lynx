// Package agui translates Lyra's internal [chat.Event] stream
// into AG-UI protocol events, using the official AG-UI Go SDK
// for the event types + serialisation.
//
// SDK: github.com/ag-ui-protocol/ag-ui/sdks/community/go
// Spec: https://docs.ag-ui.com/concepts/events
//
// The translator is the only Lyra-side piece — the wire format
// (SSE / WebSocket / IPC) is whatever the transport layer above
// chooses; for SSE callers should use the SDK's
// `pkg/encoding/sse.SSEWriter`.
package agui

import (
	"github.com/google/uuid"

	aguievents "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"

	"github.com/Tangerg/lynx/lyra/internal/service/chat"
)

// Event is the SDK's Event interface re-exported under a short
// local alias. Lets transport adapters import only this package
// when they need the union type for a channel signature.
type Event = aguievents.Event

// Translator converts Lyra's internal [chat.Event] stream into
// AG-UI events. One Translator per turn — it carries lifecycle
// state (whether an assistant message is currently open, the
// active text-message id) so the output is well-formed AG-UI
// regardless of how the underlying chat events interleave.
//
// State machine:
//
//   - chat.TurnStart      → RunStartedEvent
//   - chat.MessageDelta   → TextMessageStart (lazy) + TextMessageContent
//   - chat.ToolCallStart  → close any open text + ToolCallStart +
//                           ToolCallArgs + ToolCallEnd
//   - chat.ToolCallEnd    → ToolCallResultEvent
//   - chat.PlanGenerated  → CustomEvent(name="plan_generated")
//   - chat.ErrorEvent     → RunErrorEvent
//   - chat.TurnEnd        → close any open text + RunFinished /
//                           RunError(code=TURN_ERRORED)
type Translator struct {
	threadID string
	runID    string

	textOpen     bool
	curMessageID string
}

// NewTranslator wires a translator to a Lyra (sessionID, turnID)
// pair. The session id becomes AG-UI's threadId; the turn id
// becomes runId.
func NewTranslator(sessionID, turnID string) *Translator {
	return &Translator{threadID: sessionID, runID: turnID}
}

// Translate maps one Lyra chat event to zero or more AG-UI
// events. Returns nil when the input event has no AG-UI
// equivalent.
func (t *Translator) Translate(ev chat.Event) []Event {
	switch e := ev.(type) {
	case chat.TurnStart:
		return []Event{t.runStarted()}
	case chat.MessageDelta:
		return t.textContent(e)
	case chat.ToolCallStart:
		return t.toolCallStart(e)
	case chat.ToolCallEnd:
		return []Event{t.toolCallResult(e)}
	case chat.PlanGenerated:
		return []Event{t.planAsCustom(e)}
	case chat.ErrorEvent:
		return []Event{t.runError(e)}
	case chat.TurnEnd:
		return t.runFinishedOrErrored(e)
	}
	return nil
}

// ------------------------------------------------------------------
// per-event translators
// ------------------------------------------------------------------

func (t *Translator) runStarted() Event {
	return aguievents.NewRunStartedEvent(t.threadID, t.runID)
}

// textContent opens a text message on first delta, then emits one
// content chunk per call. Caller is the chat-event stream — if
// the model produces 200 chunks, this fires once with Start and
// 200 times with Content, all sharing one messageId.
func (t *Translator) textContent(e chat.MessageDelta) []Event {
	out := make([]Event, 0, 2)
	if !t.textOpen {
		t.curMessageID = uuid.NewString()
		t.textOpen = true
		out = append(out, aguievents.NewTextMessageStartEvent(
			t.curMessageID,
			aguievents.WithRole("assistant"),
		))
	}
	out = append(out, aguievents.NewTextMessageContentEvent(t.curMessageID, e.Text))
	return out
}

// toolCallStart closes any in-flight text message (a tool call
// interrupts the assistant's natural-language output) and then
// emits the AG-UI start/args/end triplet. Lyra knows the full
// arg JSON upfront so a single Args event suffices.
func (t *Translator) toolCallStart(e chat.ToolCallStart) []Event {
	out := make([]Event, 0, 4)
	if t.textOpen {
		out = append(out, aguievents.NewTextMessageEndEvent(t.curMessageID))
		t.textOpen = false
	}
	out = append(out,
		aguievents.NewToolCallStartEvent(e.CallID, e.ToolName,
			aguievents.WithParentMessageID(t.runID)),
		aguievents.NewToolCallArgsEvent(e.CallID, e.Arguments),
		aguievents.NewToolCallEndEvent(e.CallID),
	)
	return out
}

// toolCallResult emits AG-UI's ToolCallResult on Lyra's
// ToolCallEnd — Lyra collapses "tool finished + output" into one
// event, AG-UI separates them but the data arrives together.
func (t *Translator) toolCallResult(e chat.ToolCallEnd) Event {
	return aguievents.NewToolCallResultEvent(t.runID, e.CallID, chooseResultContent(e))
}

// chooseResultContent prefers the error message when the tool
// failed (clients show it as the result text) and falls back to
// the captured output otherwise.
func chooseResultContent(e chat.ToolCallEnd) string {
	if e.Err != "" {
		return e.Err
	}
	return e.Output
}

// planAsCustom encodes a plan-mode pause as an AG-UI CustomEvent.
// AG-UI v1 has no first-class plan event; "plan_generated" is
// Lyra's convention — the frontend already coordinates on it.
func (t *Translator) planAsCustom(e chat.PlanGenerated) Event {
	return aguievents.NewCustomEvent("plan_generated",
		aguievents.WithValue(map[string]any{
			"runId": t.runID,
			"plan":  e.Plan,
		}),
	)
}

func (t *Translator) runError(e chat.ErrorEvent) Event {
	opts := []aguievents.RunErrorOption{aguievents.WithRunID(t.runID)}
	if e.Code != "" {
		opts = append(opts, aguievents.WithErrorCode(e.Code))
	}
	return aguievents.NewRunErrorEvent(e.Message, opts...)
}

// runFinishedOrErrored closes the run, also closing any in-flight
// text message. TurnEndErrored produces a RunErrorEvent (in
// addition to any chat.ErrorEvent already emitted) so AG-UI
// clients see one terminal event regardless of which Lyra path
// got here.
func (t *Translator) runFinishedOrErrored(e chat.TurnEnd) []Event {
	out := make([]Event, 0, 2)
	if t.textOpen {
		out = append(out, aguievents.NewTextMessageEndEvent(t.curMessageID))
		t.textOpen = false
	}
	if e.Reason == chat.TurnEndErrored {
		out = append(out, aguievents.NewRunErrorEvent("turn errored",
			aguievents.WithErrorCode("TURN_ERRORED"),
			aguievents.WithRunID(t.runID),
		))
		return out
	}
	out = append(out, aguievents.NewRunFinishedEvent(t.threadID, t.runID))
	return out
}
