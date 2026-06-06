package engine

import (
	"encoding/json"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/model/chat"
)

// inflightTailKey holds, on the process blackboard, the resumable tail a HITL
// interrupt parks: the interrupting round's assistant tool-call message plus
// any partial tool results, as a FinishReasonInterrupt response carried up by
// the tool loop. HITL resume runs on the SAME process; the tail rides the same
// blackboard across the suspend → ResumeProcess → re-tick cycle, and on resume
// is fed back so the loop continues AT the still-pending call (the model is NOT
// re-invoked for that round).
//
// It is stored as the chat-message discriminated JSON (a string), not a raw
// []chat.Message — a string round-trips the blackboard process snapshot, so a
// cross-restart resume (rebuilding the process from that snapshot) still finds
// the tail. A raw interface slice would not survive the generic snapshot.
const inflightTailKey = "lyra:hitl:inflight-tail"

// isInterruptResult reports whether a streamed response is the tool loop's
// FinishReasonInterrupt tail (assistant + partial results) rather than model
// output — the signal to park, not to render.
func isInterruptResult(resp *chat.Response) bool {
	return resp != nil && resp.Result != nil && resp.Result.Metadata != nil &&
		resp.Result.Metadata.FinishReason == chat.FinishReasonInterrupt
}

// inflightTailStore is the keyed access to the HITL inflight tail on a
// process blackboard. It owns the [inflightTailKey] convention and the
// string-encoded serialization format ([marshalMessages]) in one place, so
// the save → load → clear cycle the resume path drives reads as one object
// rather than three blackboard pokes at a shared magic key.
type inflightTailStore struct {
	bb core.Blackboard
}

// Save parks the interrupt response's tail (assistant tool-call message + any
// partial tool results) for the resuming re-tick to feed back. No-op when the
// result carries no assistant message or fails to encode.
func (s inflightTailStore) Save(result *chat.Result) {
	if result == nil || result.AssistantMessage == nil {
		return
	}
	tail := []chat.Message{result.AssistantMessage}
	if result.ToolMessage != nil {
		tail = append(tail, result.ToolMessage)
	}
	data, err := marshalMessages(tail)
	if err != nil {
		return
	}
	s.bb.Set(inflightTailKey, data)
}

// Load returns the parked tail, or (nil, false) when none is set.
func (s inflightTailStore) Load() ([]chat.Message, bool) {
	v, ok := s.bb.Get(inflightTailKey)
	if !ok {
		return nil, false
	}
	data, ok := v.(string)
	if !ok || data == "" {
		return nil, false
	}
	msgs, err := unmarshalMessages(data)
	if err != nil || len(msgs) == 0 {
		return nil, false
	}
	return msgs, true
}

// Clear drops a consumed tail (the blackboard has no delete; Load treats
// empty as absent).
func (s inflightTailStore) Clear() {
	s.bb.Set(inflightTailKey, "")
}

// marshalMessages encodes messages as a JSON array of the per-message
// discriminated form ([chat] each message MarshalJSON tags its "type"), so the
// slice round-trips a generic snapshot as a plain string.
func marshalMessages(msgs []chat.Message) (string, error) {
	raws := make([]json.RawMessage, 0, len(msgs))
	for _, m := range msgs {
		b, err := json.Marshal(m)
		if err != nil {
			return "", err
		}
		raws = append(raws, b)
	}
	b, err := json.Marshal(raws)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// unmarshalMessages reverses [marshalMessages] via [chat.UnmarshalMessage],
// which dispatches each element on its "type" discriminator back to the
// concrete message type.
func unmarshalMessages(data string) ([]chat.Message, error) {
	var raws []json.RawMessage
	if err := json.Unmarshal([]byte(data), &raws); err != nil {
		return nil, err
	}
	msgs := make([]chat.Message, 0, len(raws))
	for _, raw := range raws {
		m, err := chat.UnmarshalMessage(raw)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, nil
}

// InterruptResolution is the human's structured answer to a HITL interrupt
// — the resume value carried back into the run (the typed R of
// hitl.Interrupt). It is deliberately a rich, protocol-conforming shape
// rather than a bare bool/string, so one resolution type serves every HITL
// flavor: tool-call approval (Approved, plus optionally Arguments to run an
// edited call), plan review (Approved), and asking the user a question
// (Answer). The RPC layer maps protocol.InterruptResponse onto this; the
// gate / ask_user tool / plan gate read the field they care about.
type InterruptResolution struct {
	// Approved is the approve/deny decision for approval + plan-review
	// interrupts. Ignored for pure question interrupts.
	Approved bool

	// Arguments, when non-empty, replaces the tool call's arguments before
	// it runs — the "approve, but with these edits" HITL affordance. JSON,
	// same shape the model produced. Empty means run the call unchanged.
	Arguments string

	// Answer carries a structured reply for question / ask_user interrupts
	// (the protocol answers map). Nil for approval-only resolutions.
	Answer map[string]any
}
