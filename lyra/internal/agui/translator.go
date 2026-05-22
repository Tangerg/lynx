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
// state (an in-flight assistant message, the active text-message
// id) so the output is well-formed AG-UI regardless of how the
// underlying chat events interleave.
//
// State machine:
//
//   - chat.TurnStart       → RunStartedEvent
//   - chat.MessageDelta    → TextMessageStart (lazy) + TextMessageContent
//   - chat.ReasoningDelta  → ThinkingTextMessageStart (lazy) +
//                            ThinkingTextMessageContent
//   - chat.ToolCallStart   → close any open text + ToolCallStart +
//                            ToolCallArgs + ToolCallEnd
//   - chat.ToolCallEnd     → ToolCallResultEvent
//   - chat.PlanGenerated   → CustomEvent(name="plan_generated")
//   - chat.ErrorEvent      → RunErrorEvent
//   - chat.TurnEnd         → close any open text + RunFinished /
//                            RunError(code=TURN_ERRORED)
type Translator struct {
	threadID  string
	runID     string
	text      textStream
	reasoning reasoningStream
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
		return []Event{aguievents.NewRunStartedEvent(t.threadID, t.runID)}
	case chat.MessageDelta:
		out := t.reasoning.closeIfOpen()
		return append(out, t.text.appendDelta(e.Text)...)
	case chat.ReasoningDelta:
		return t.reasoning.appendDelta(e.Text)
	case chat.ToolCallStart:
		return t.toolCallStart(e)
	case chat.ToolCallEnd:
		return []Event{t.toolCallResult(e)}
	case chat.PlanGenerated:
		return []Event{t.planAsCustom(e)}
	case chat.ToolCallApproval:
		return []Event{t.approvalAsCustom(e)}
	case chat.ErrorEvent:
		return []Event{t.runError(e)}
	case chat.TurnEnd:
		return t.runFinishedOrErrored(e)
	}
	return nil
}

// toolCallStart closes any in-flight text / reasoning message (a
// tool call interrupts both) and then emits the AG-UI
// start/args/end triplet. Lyra knows the full arg JSON upfront so
// a single Args event suffices.
func (t *Translator) toolCallStart(e chat.ToolCallStart) []Event {
	out := t.reasoning.closeIfOpen()
	out = append(out, t.text.closeIfOpen()...)
	return append(out,
		aguievents.NewToolCallStartEvent(e.CallID, e.ToolName,
			aguievents.WithParentMessageID(t.runID)),
		aguievents.NewToolCallArgsEvent(e.CallID, e.Arguments),
		aguievents.NewToolCallEndEvent(e.CallID),
	)
}

// toolCallResult emits AG-UI's ToolCallResult on Lyra's
// ToolCallEnd — Lyra collapses "tool finished + output" into one
// event, AG-UI separates them but the data arrives together.
// Failed tools surface their error message as the Content so
// AG-UI clients render the failure verbatim.
func (t *Translator) toolCallResult(e chat.ToolCallEnd) Event {
	content := e.Output
	if e.Err != "" {
		content = e.Err
	}
	return aguievents.NewToolCallResultEvent(t.runID, e.CallID, content)
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

// approvalAsCustom encodes an approval-pause as an AG-UI
// CustomEvent. Like plan_generated, "tool_call_approval" is a
// Lyra convention — the frontend reads it, prompts the user,
// then POSTs the verdict to /v1/approvals/{id}.
func (t *Translator) approvalAsCustom(e chat.ToolCallApproval) Event {
	return aguievents.NewCustomEvent("tool_call_approval",
		aguievents.WithValue(map[string]any{
			"runId":     t.runID,
			"requestId": e.Request.ID,
			"toolName":  e.Request.ToolName,
			"arguments": e.Request.Arguments,
		}),
	)
}

// runError lifts a chat.ErrorEvent into an AG-UI RunErrorEvent,
// preserving the stable Code when present so transports / UIs
// can branch on it.
func (t *Translator) runError(e chat.ErrorEvent) Event {
	opts := []aguievents.RunErrorOption{aguievents.WithRunID(t.runID)}
	if e.Code != "" {
		opts = append(opts, aguievents.WithErrorCode(e.Code))
	}
	return aguievents.NewRunErrorEvent(e.Message, opts...)
}

// runFinishedOrErrored closes the run, also closing any in-flight
// text / reasoning message. TurnEndErrored produces a
// RunErrorEvent (in addition to any chat.ErrorEvent already
// emitted) so AG-UI clients see one terminal event regardless of
// which Lyra path got here.
func (t *Translator) runFinishedOrErrored(e chat.TurnEnd) []Event {
	out := t.reasoning.closeIfOpen()
	out = append(out, t.text.closeIfOpen()...)
	if e.Reason == chat.TurnEndErrored {
		return append(out, aguievents.NewRunErrorEvent("turn errored",
			aguievents.WithErrorCode("TURN_ERRORED"),
			aguievents.WithRunID(t.runID),
		))
	}
	return append(out, aguievents.NewRunFinishedEvent(t.threadID, t.runID))
}

// ------------------------------------------------------------------
// textStream — the in-flight assistant message's own state
// ------------------------------------------------------------------

// textStream tracks one streaming TextMessage's lifecycle: whether
// it's open, and the messageId that ties Start / Content / End
// together on the AG-UI wire.
//
// Splitting it out of [Translator] keeps the "is a text message
// open?" question in one place — Translator's other branches just
// call closeIfOpen before they emit non-text events instead of
// each tracking the bool themselves.
type textStream struct {
	open      bool
	messageID string
}

// appendDelta returns the AG-UI events for one [chat.MessageDelta].
// First call opens the message and emits Start + Content; later
// calls emit only Content, all sharing the original messageId.
func (s *textStream) appendDelta(text string) []Event {
	if !s.open {
		s.messageID = uuid.NewString()
		s.open = true
		return []Event{
			aguievents.NewTextMessageStartEvent(s.messageID, aguievents.WithRole("assistant")),
			aguievents.NewTextMessageContentEvent(s.messageID, text),
		}
	}
	return []Event{aguievents.NewTextMessageContentEvent(s.messageID, text)}
}

// closeIfOpen returns a one-element slice containing the
// TextMessageEnd event when a stream is open (and flips the
// state to closed); nil otherwise. Returning a slice (not the
// event directly) lets callers append the rest of their per-event
// output with a single append — no nil-check at the call site.
func (s *textStream) closeIfOpen() []Event {
	if !s.open {
		return nil
	}
	end := aguievents.NewTextMessageEndEvent(s.messageID)
	s.open = false
	return []Event{end}
}

// ------------------------------------------------------------------
// reasoningStream — mirrors textStream for extended-thinking
// chunks, emitting the AG-UI ThinkingStart / ThinkingTextMessage*
// / ThinkingEnd lifecycle. Splitting from textStream keeps each
// state machine focused on one event family.
// ------------------------------------------------------------------

type reasoningStream struct {
	open bool
}

// appendDelta opens the thinking lifecycle on first call (Start +
// MessageStart + MessageContent) and emits Content only on
// follow-ups. The AG-UI thinking events don't carry an explicit
// message id — the protocol scopes them to the run.
func (s *reasoningStream) appendDelta(text string) []Event {
	if !s.open {
		s.open = true
		return []Event{
			aguievents.NewThinkingStartEvent(),
			aguievents.NewThinkingTextMessageStartEvent(),
			aguievents.NewThinkingTextMessageContentEvent(text),
		}
	}
	return []Event{aguievents.NewThinkingTextMessageContentEvent(text)}
}

// closeIfOpen finalises the thinking lifecycle: MessageEnd +
// ThinkingEnd. Returns nil when no thinking message is in flight
// so callers can append unconditionally.
func (s *reasoningStream) closeIfOpen() []Event {
	if !s.open {
		return nil
	}
	s.open = false
	return []Event{
		aguievents.NewThinkingTextMessageEndEvent(),
		aguievents.NewThinkingEndEvent(),
	}
}
