package engine

import (
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/model/chat"
)

// inflightConversationKey is the blackboard slot holding a turn's in-flight
// conversation across a HITL interrupt — saved when the run parks, consumed
// when the continuation run resumes. One process (one blackboard) per turn,
// so a single name binding suffices; it rides the same in-memory blackboard
// across the suspend → ResumeProcess → re-tick cycle. Cross-restart rebuilds
// the blackboard from a JSON snapshot where the message-typed slot does not
// round-trip; loadInflightConversation then reports absent and the turn
// falls back to a full re-run (rare, best-effort).
const inflightConversationKey = "lyra:hitl:inflight-conversation"

// loadInflightConversation returns the saved in-flight conversation, or
// (nil, false) when none is parked.
func loadInflightConversation(bb core.Blackboard) ([]chat.Message, bool) {
	v, ok := bb.Get(inflightConversationKey)
	if !ok {
		return nil, false
	}
	msgs, ok := v.([]chat.Message)
	if !ok || len(msgs) == 0 {
		return nil, false
	}
	return msgs, true
}

// saveInflightConversation parks the conversation the chat tool loop
// carried up on interrupt, so the continuation run resumes from it.
func saveInflightConversation(bb core.Blackboard, msgs []chat.Message) {
	bb.Set(inflightConversationKey, msgs)
}

// clearInflightConversation drops a consumed conversation (the blackboard
// has no delete; load treats nil/empty as absent).
func clearInflightConversation(bb core.Blackboard) {
	bb.Set(inflightConversationKey, ([]chat.Message)(nil))
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
