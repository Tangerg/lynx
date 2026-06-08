package server

import (
	"time"

	"github.com/Tangerg/lynx/lyra/internal/engine"
	"github.com/Tangerg/lynx/lyra/internal/service/chat"
	"github.com/Tangerg/lynx/lyra/rpc/protocol"
)

// interrupt maps a parked turn (HITL) onto its Item(s) + a terminal
// run.finished{outcome:interrupt}. Each pending awaitable becomes a
// durable Item the client renders plus a protocol.Interrupt keyed by
// that item's id:
//
//	approval → a toolCall Item (inProgress) for the gated call
//	question → a question Item (inProgress) for a plan awaiting review
//
// (The contract has no "plan" interrupt kind — plan-review rides the
// generic question mechanism; see questionInterrupt.)
func (t *translator) interrupt(e chat.TurnInterrupted) []protocol.StreamEvent {
	out := t.closeReasoning()
	out = append(out, t.closeText()...)
	// Close any tool item still open when the turn parks (defensive: the
	// gated call itself paused before item.started, but a sibling tool could
	// be mid-flight) so no started item is left without a terminal (§5.2).
	out = append(out, t.drainTools()...)

	wire := make([]protocol.Interrupt, 0, len(e.Interrupts))
	for _, in := range e.Interrupts {
		var ev protocol.StreamEvent
		var entry protocol.Interrupt
		switch in.Kind {
		case "approval":
			ev, entry = t.approvalInterrupt(in)
		default: // question — ask_user (free text) or plan review (choice)
			if _, ok := in.Payload.(engine.QuestionPrompt); ok {
				ev, entry = t.askUserInterrupt(in)
			} else {
				ev, entry = t.questionInterrupt(in)
			}
		}
		out = append(out, ev)
		wire = append(wire, entry)
	}

	return append(out, protocol.StreamEvent{
		Type:    protocol.StreamRunFinished,
		Outcome: &protocol.RunOutcome{Type: protocol.OutcomeInterrupt, Interrupts: wire},
	})
}

// approvalInterrupt renders a gated tool call awaiting approval as an
// inProgress toolCall Item plus the protocol.Interrupt keyed to it.
func (t *translator) approvalInterrupt(in chat.Interrupt) (protocol.StreamEvent, protocol.Interrupt) {
	p, _ := in.Payload.(chat.ApprovalPrompt)
	id := t.nextItemID()
	// The gated tool as a full ToolInvocation (arguments parsed, no result
	// yet). The approval Interrupt's payload reuses it (API.md §4.8:
	// ApprovalPayload.tool), so the client reads payload.tool directly
	// instead of guessing where the command / args live.
	inv := toolInvocation(p.ToolName, p.Arguments, "")
	ev := protocol.StreamEvent{
		Type: protocol.StreamItemStarted,
		Item: &protocol.Item{
			ID:        id,
			RunID:     t.runID,
			Status:    protocol.ItemStatusRunning,
			Type:      protocol.ItemTypeToolCall,
			CreatedAt: time.Now().UTC(),
			Tool:      inv,
		},
	}
	entry := protocol.Interrupt{
		ItemID: id,
		Type:   "approval",
		// payload.tool is the self-contained ApprovalPayload (API.md §4.8): the
		// domain-neutral ToolInvocation always carries name + arguments, so the
		// server re-binds the re-fired approved tool to THIS proposal item
		// across the resume boundary straight off payload.tool (resumeKey on
		// name + canonical arguments) — no backend-internal `_resume` tuple.
		Payload: map[string]any{"tool": inv},
	}
	return ev, entry
}

// Plan-review interrupt shape. The contract has no "plan" interrupt kind
// (API.md §6: approval | question | toolResult), so a plan awaiting review
// surfaces through the generic question mechanism: an inProgress question
// Item whose prompt is the plan markdown and whose single choice field
// decides approve / reject. These constants are the single source the
// resume path (resolveDecision) reads the answer back against.
const (
	planDecisionField   = "decision"
	planDecisionApprove = "Approve"
	planDecisionReject  = "Reject"
)

// questionInterrupt renders a plan awaiting review as an inProgress
// question Item (the plan markdown as the prompt, an Approve/Reject choice)
// plus the protocol.Interrupt keyed to it. The client answers via
// runs.resume with an "answer" response carrying the chosen label.
func (t *translator) questionInterrupt(in chat.Interrupt) (protocol.StreamEvent, protocol.Interrupt) {
	plan, _ := in.Payload.(string)
	id := t.nextItemID()
	question := &protocol.Question{
		Prompt: plan,
		Fields: []protocol.QuestionField{{
			Name:     planDecisionField,
			Label:    "Proceed with this plan?",
			Header:   "Plan",
			Required: true,
			Type:     "choice",
			Options: []protocol.QuestionOption{
				{Label: planDecisionApprove},
				{Label: planDecisionReject},
			},
		}},
	}
	ev := protocol.StreamEvent{
		Type: protocol.StreamItemStarted,
		Item: &protocol.Item{
			ID:        id,
			RunID:     t.runID,
			Status:    protocol.ItemStatusRunning,
			Type:      protocol.ItemTypeQuestion,
			CreatedAt: time.Now().UTC(),
			Question:  question,
		},
	}
	entry := protocol.Interrupt{
		ItemID:  id,
		Type:    "question",
		Payload: map[string]any{"question": question},
	}
	return ev, entry
}

// askUserQuestionField is the free-text answer field name the ask_user
// tool reads back (engine.answerText keys on "text"); the resume "answer"
// response carries answers["text"].
const askUserQuestionField = "text"

// askUserInterrupt renders the model's ask_user call as an inProgress
// question Item carrying the actual question + a single free-text answer
// field (vs. questionInterrupt's plan Approve/Reject choice). The client
// answers via runs.resume with an "answer" response carrying the text.
func (t *translator) askUserInterrupt(in chat.Interrupt) (protocol.StreamEvent, protocol.Interrupt) {
	q, _ := in.Payload.(engine.QuestionPrompt)
	id := t.nextItemID()
	question := &protocol.Question{
		Prompt: q.Question,
		Fields: []protocol.QuestionField{{
			Name:     askUserQuestionField,
			Label:    q.Question,
			Required: true,
			Type:     "text",
		}},
	}
	ev := protocol.StreamEvent{
		Type: protocol.StreamItemStarted,
		Item: &protocol.Item{
			ID:        id,
			RunID:     t.runID,
			Status:    protocol.ItemStatusRunning,
			Type:      protocol.ItemTypeQuestion,
			CreatedAt: time.Now().UTC(),
			Question:  question,
		},
	}
	entry := protocol.Interrupt{
		ItemID:  id,
		Type:    "question",
		Payload: map[string]any{"question": question},
	}
	return ev, entry
}
