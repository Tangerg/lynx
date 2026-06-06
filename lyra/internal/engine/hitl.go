package engine

import (
	"github.com/Tangerg/lynx/agent/core"
)

// turnSeededKey marks, on the process blackboard, that the turn's user
// message has been added to the chat request (and thus persisted to memory by
// the inner memory middleware). HITL resume re-runs the turn on the SAME
// process; the marker rides the same blackboard across the
// suspend → ResumeProcess → re-tick cycle, so the re-run observes it and
// skips re-adding the user message — re-adding would duplicate it in the
// stored history. Unlike a message slice, a bool round-trips a blackboard
// JSON snapshot, so a cross-restart resume still observes it.
const turnSeededKey = "lyra:hitl:turn-seeded"

// turnSeeded reports whether this turn's user message has already been added
// (so a HITL resume re-run must not add it again).
func turnSeeded(bb core.Blackboard) bool {
	v, ok := bb.Get(turnSeededKey)
	if !ok {
		return false
	}
	seeded, _ := v.(bool)
	return seeded
}

// seedTurn records that the turn's user message has been added.
func seedTurn(bb core.Blackboard) {
	bb.Set(turnSeededKey, true)
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
