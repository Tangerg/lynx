// Package agui translates Lyra's internal [chat.Event] stream
// into AG-UI protocol events, using the official AG-UI Go SDK
// for the event types + serialization.
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
// state (in-flight assistant message, in-flight thinking message,
// the active tool-call step names) so the output is well-formed
// AG-UI regardless of how the underlying chat events interleave.
//
// State machine:
//
//   - chat.TurnStart        → RunStartedEvent
//   - chat.MessageDelta     → close thinking + TextMessageStart (lazy) +
//     TextMessageContent
//   - chat.ReasoningDelta   → ThinkingStart (lazy) +
//     ThinkingTextMessageStart (lazy) +
//     ThinkingTextMessageContent
//   - chat.ToolCallStart    → close thinking + close text +
//     StepStarted("tool:<name>") +
//     ToolCallStart + ToolCallArgs +
//     ToolCallEnd
//   - chat.ToolCallEnd      → ToolCallResultEvent +
//     StepFinished("tool:<name>")
//   - chat.PlanGenerated    → StepStarted("plan_review") +
//     CustomEvent("plan_generated") +
//     StepFinished("plan_review")
//   - chat.ToolCallApproval → StepStarted("approval:<name>") +
//     CustomEvent("tool_call_approval")
//     (StepFinished fires next ToolCall*)
//   - chat.ErrorEvent       → RunErrorEvent
//   - chat.TurnEnd          → close streams + RunFinished /
//     RunError(code=TURN_ERRORED)
type Translator struct {
	threadID  string
	runID     string
	text      textStream
	reasoning reasoningStream

	// toolSteps remembers the StepStarted name we emitted for each
	// in-flight tool call so the matching StepFinished can use the
	// same name. Keyed by callID; entry removed on ToolCallEnd.
	toolSteps map[string]string

	// approvalSteps tracks the same for tool-call-approval pauses,
	// keyed by the approval request id (same as the tool callID).
	// StepFinished fires when the matching tool actually starts /
	// ends (denial flows back as a ToolCallEnd with the denial
	// text), or at TurnEnd as a safety drain.
	approvalSteps map[string]string
}

// NewTranslator wires a translator to a Lyra (sessionID, turnID)
// pair. The session id becomes AG-UI's threadId; the turn id
// becomes runId.
func NewTranslator(sessionID, turnID string) *Translator {
	return &Translator{
		threadID:      sessionID,
		runID:         turnID,
		toolSteps:     map[string]string{},
		approvalSteps: map[string]string{},
	}
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
		return t.toolCallResult(e)
	case chat.PlanGenerated:
		return t.planAsCustom(e)
	case chat.ToolCallApproval:
		return t.approvalAsCustom(e)
	case chat.CompactBoundary:
		return t.maintenanceCustom("compact_boundary", map[string]any{
			"runId":          t.runID,
			"messagesBefore": e.MessagesBefore,
			"messagesAfter":  e.MessagesAfter,
		})
	case chat.MemoryUpdated:
		return t.maintenanceCustom("memory_updated", map[string]any{
			"runId": t.runID,
			"facts": e.Facts,
		})
	case chat.ErrorEvent:
		return []Event{t.runError(e)}
	case chat.TurnEnd:
		return t.runFinishedOrErrored(e)
	}
	return nil
}

// toolCallStart closes any in-flight text / reasoning message (a
// tool call interrupts both) and then emits the AG-UI step + tool
// lifecycle. A matching StepFinished fires from [toolCallResult]
// when the tool returns so clients can render "step in progress"
// indicators. The step name uses the prefix convention
// "tool:<name>" so multiple Steps in one turn stay separable on
// the wire.
//
// If an approval Step was open for this callID (model wanted to
// run a gated tool and the user approved), the approval Step
// finishes here — execution has begun.
func (t *Translator) toolCallStart(e chat.ToolCallStart) []Event {
	out := t.reasoning.closeIfOpen()
	out = append(out, t.text.closeIfOpen()...)
	if name, ok := t.approvalSteps[e.CallID]; ok {
		out = append(out, aguievents.NewStepFinishedEvent(name))
		delete(t.approvalSteps, e.CallID)
	}
	stepName := "tool:" + e.ToolName
	t.toolSteps[e.CallID] = stepName
	return append(out,
		aguievents.NewStepStartedEvent(stepName),
		aguievents.NewToolCallStartEvent(e.CallID, e.ToolName,
			aguievents.WithParentMessageID(t.runID)),
		aguievents.NewToolCallArgsEvent(e.CallID, e.Arguments),
		aguievents.NewToolCallEndEvent(e.CallID),
	)
}

// toolCallResult emits AG-UI's ToolCallResult on Lyra's
// ToolCallEnd and closes the matching Step. Lyra collapses "tool
// finished + output" into one event, AG-UI separates them but the
// data arrives together. Failed tools surface their error message
// as the Content so AG-UI clients render the failure verbatim.
//
// Edge case: a denied approval emits ToolCallEnd without a prior
// ToolCallStart (the engine short-circuits the tool). The
// approval Step (if any) is closed here so clients still see a
// matching Started/Finished pair.
func (t *Translator) toolCallResult(e chat.ToolCallEnd) []Event {
	content := e.Output
	if e.Err != "" {
		content = e.Err
	}
	out := []Event{aguievents.NewToolCallResultEvent(t.runID, e.CallID, content)}
	if name, ok := t.toolSteps[e.CallID]; ok {
		out = append(out, aguievents.NewStepFinishedEvent(name))
		delete(t.toolSteps, e.CallID)
	}
	if name, ok := t.approvalSteps[e.CallID]; ok {
		out = append(out, aguievents.NewStepFinishedEvent(name))
		delete(t.approvalSteps, e.CallID)
	}
	return out
}

// planAsCustom encodes a plan-mode pause as a Step-bracketed
// CustomEvent. The Step's Start/Finish straddle the Custom so
// clients can render plan-review as a discrete phase; the Custom
// is the part they actually display. AG-UI v1 has no first-class
// plan event — "plan_generated" is Lyra's convention.
func (t *Translator) planAsCustom(e chat.PlanGenerated) []Event {
	const stepName = "plan_review"
	return []Event{
		aguievents.NewStepStartedEvent(stepName),
		aguievents.NewCustomEvent("plan_generated",
			aguievents.WithValue(map[string]any{
				"runId": t.runID,
				"plan":  e.Plan,
			}),
		),
		aguievents.NewStepFinishedEvent(stepName),
	}
}

// approvalAsCustom encodes an approval-pause as a Step-bracketed
// CustomEvent. The Step is left OPEN until [toolCallStart] or
// [toolCallResult] closes it — the approval decision is what
// resolves the step, not the event itself. Like plan_generated,
// "tool_call_approval" is a Lyra convention.
func (t *Translator) approvalAsCustom(e chat.ToolCallApproval) []Event {
	stepName := "approval:" + e.Request.ToolName
	t.approvalSteps[e.Request.ID] = stepName
	return []Event{
		aguievents.NewStepStartedEvent(stepName),
		aguievents.NewCustomEvent("tool_call_approval",
			aguievents.WithValue(map[string]any{
				"runId":     t.runID,
				"requestId": e.Request.ID,
				"toolName":  e.Request.ToolName,
				"arguments": e.Request.Arguments,
			}),
		),
	}
}

// maintenanceCustom encodes a post-turn housekeeping event
// (compaction boundary, memory update) as a standalone AG-UI
// CustomEvent. It first closes any in-flight text / reasoning
// stream — these fire after the assistant's reply is complete, so
// the streams are done — keeping the wire balanced (every Start has
// its End before the Custom lands). No Step bracket: these are
// instantaneous notifications, not interactive phases.
func (t *Translator) maintenanceCustom(name string, value map[string]any) []Event {
	out := t.reasoning.closeIfOpen()
	out = append(out, t.text.closeIfOpen()...)
	return append(out, aguievents.NewCustomEvent(name, aguievents.WithValue(value)))
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

// runFinishedOrErrored closes the run, also draining any open
// streams or step lifecycles so the wire ends in a balanced state
// (every Start has a Finish). TurnEndErrored produces a
// RunErrorEvent (in addition to any chat.ErrorEvent already
// emitted) so AG-UI clients see one terminal event regardless of
// which Lyra path got here.
func (t *Translator) runFinishedOrErrored(e chat.TurnEnd) []Event {
	out := t.reasoning.closeIfOpen()
	out = append(out, t.text.closeIfOpen()...)
	out = append(out, t.drainOpenSteps()...)
	if e.Reason == chat.TurnEndErrored {
		return append(out, aguievents.NewRunErrorEvent("turn errored",
			aguievents.WithErrorCode("TURN_ERRORED"),
			aguievents.WithRunID(t.runID),
		))
	}
	return append(out, aguievents.NewRunFinishedEvent(t.threadID, t.runID))
}

// drainOpenSteps emits a StepFinished for every Step the
// translator still has open — orphaned tool calls or approval
// pauses that the turn ended without resolving. Clients that
// require balanced Start/Finish pairs (e.g. progress UIs) stay
// happy.
func (t *Translator) drainOpenSteps() []Event {
	if len(t.toolSteps) == 0 && len(t.approvalSteps) == 0 {
		return nil
	}
	out := make([]Event, 0, len(t.toolSteps)+len(t.approvalSteps))
	for callID, name := range t.toolSteps {
		out = append(out, aguievents.NewStepFinishedEvent(name))
		delete(t.toolSteps, callID)
	}
	for reqID, name := range t.approvalSteps {
		out = append(out, aguievents.NewStepFinishedEvent(name))
		delete(t.approvalSteps, reqID)
	}
	return out
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

// closeIfOpen finalizes the thinking lifecycle: MessageEnd +
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
