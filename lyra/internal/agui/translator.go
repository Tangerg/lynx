package agui

import (
	"time"

	"github.com/google/uuid"

	"github.com/Tangerg/lynx/lyra/internal/service/chat"
)

// Translator converts Lyra's internal [chat.Event] stream into
// AG-UI events. One Translator per turn — it carries lifecycle
// state (whether an assistant message is currently open, the
// active text-message id) so the output is well-formed AG-UI
// regardless of how the underlying chat events interleave.
//
// The translator does NOT own the underlying chat.Event channel;
// transport adapters call [Translator.Translate] per event and
// fan out the returned slice on whatever wire they speak.
//
// State machine:
//
//   - TurnStart       → RunStarted (lazily; first event of the run)
//   - MessageDelta    → TextMessageStart (lazy) + TextMessageContent
//   - ToolCallStart   → close any open text + ToolCallStart +
//                       ToolCallArgs + ToolCallEnd
//   - ToolCallEnd     → ToolCallResult
//   - PlanGenerated   → Custom(name="plan_generated")
//   - ErrorEvent      → RunError
//   - TurnEnd         → close any open text + RunFinished / RunError
//
// AG-UI requires TextMessageStart / End to wrap streamed content;
// the translator emits them at the right boundaries so callers
// never have to think about it.
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
		return []Event{t.runStarted(e)}
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

func (t *Translator) runStarted(e chat.TurnStart) Event {
	return RunStarted{
		Type:      "RunStarted",
		Timestamp: e.Timestamp,
		ThreadID:  t.threadID,
		RunID:     t.runID,
	}
}

// textContent opens a text message on first delta, then emits one
// content chunk per call. Caller is the chat-event stream — if
// the model produces 200 chunks, this fires once with Start and
// 200 times with Content.
func (t *Translator) textContent(e chat.MessageDelta) []Event {
	out := make([]Event, 0, 2)
	if !t.textOpen {
		t.curMessageID = uuid.NewString()
		t.textOpen = true
		out = append(out, TextMessageStart{
			Type:      "TextMessageStart",
			Timestamp: e.Timestamp,
			MessageID: t.curMessageID,
			Role:      "assistant",
		})
	}
	out = append(out, TextMessageContent{
		Type:      "TextMessageContent",
		Timestamp: e.Timestamp,
		MessageID: t.curMessageID,
		Delta:     e.Text,
	})
	return out
}

// toolCallStart closes any in-flight text message (a tool call
// interrupts the assistant's natural-language output) and then
// emits the AG-UI start/args/end triplet. Lyra knows the full
// arg JSON upfront so a single Args event suffices.
func (t *Translator) toolCallStart(e chat.ToolCallStart) []Event {
	out := make([]Event, 0, 4)
	if t.textOpen {
		out = append(out, TextMessageEnd{
			Type:      "TextMessageEnd",
			Timestamp: e.Timestamp,
			MessageID: t.curMessageID,
		})
		t.textOpen = false
	}
	out = append(out,
		ToolCallStart{
			Type:            "ToolCallStart",
			Timestamp:       e.Timestamp,
			ToolCallID:      e.CallID,
			ToolCallName:    e.ToolName,
			ParentMessageID: t.runID,
		},
		ToolCallArgs{
			Type:       "ToolCallArgs",
			Timestamp:  e.Timestamp,
			ToolCallID: e.CallID,
			Delta:      e.Arguments,
		},
		ToolCallEnd{
			Type:       "ToolCallEnd",
			Timestamp:  e.Timestamp,
			ToolCallID: e.CallID,
		},
	)
	return out
}

// toolCallResult emits AG-UI's ToolCallResult on Lyra's
// ToolCallEnd — Lyra collapses "tool finished + output" into one
// event, AG-UI separates them but the data arrives together.
func (t *Translator) toolCallResult(e chat.ToolCallEnd) Event {
	return ToolCallResult{
		Type:       "ToolCallResult",
		Timestamp:  e.Timestamp,
		MessageID:  t.runID,
		ToolCallID: e.CallID,
		Content:    chooseResultContent(e),
		Role:       "tool",
	}
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

// planAsCustom encodes a plan-mode pause as an AG-UI Custom
// event. AG-UI v1 has no first-class plan event; "plan_generated"
// is Lyra's convention — the frontend already knows the name
// because the two repos coordinated on it.
func (t *Translator) planAsCustom(e chat.PlanGenerated) Event {
	return Custom{
		Type:      "Custom",
		Timestamp: e.Timestamp,
		Name:      "plan_generated",
		Value: map[string]any{
			"runId": t.runID,
			"plan":  e.Plan,
		},
	}
}

func (t *Translator) runError(e chat.ErrorEvent) Event {
	return RunError{
		Type:      "RunError",
		Timestamp: e.Timestamp,
		Message:   e.Message,
		Code:      e.Code,
	}
}

// runFinishedOrErrored closes the run, also closing any in-flight
// text message. TurnEndErrored produces a RunError (in addition
// to any ErrorEvent already emitted) so AG-UI clients see one
// terminal event regardless of which Lyra path got here.
func (t *Translator) runFinishedOrErrored(e chat.TurnEnd) []Event {
	out := make([]Event, 0, 2)
	if t.textOpen {
		out = append(out, TextMessageEnd{
			Type:      "TextMessageEnd",
			Timestamp: e.Timestamp,
			MessageID: t.curMessageID,
		})
		t.textOpen = false
	}
	if e.Reason == chat.TurnEndErrored {
		out = append(out, RunError{
			Type:      "RunError",
			Timestamp: e.Timestamp,
			Message:   "turn errored",
			Code:      "TURN_ERRORED",
		})
		return out
	}
	out = append(out, RunFinished{
		Type:      "RunFinished",
		Timestamp: e.Timestamp,
	})
	return out
}

// Now is the time source used by tests to assert deterministic
// timestamps. Production passes through to time.Now via the
// chat.Event's own timestamp; this hook is reserved for cases
// where the translator might mint its own (none today).
var _ = time.Now
