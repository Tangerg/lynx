package server

import (
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
)

// interrupt maps a parked turn (HITL) onto its Item(s) + a terminal
// run.finished{outcome:interrupt}. Each pending awaitable becomes a
// durable Item the client renders plus a protocol.Interrupt keyed by
// that item's id:
//
//	approval → a toolCall Item (inProgress) for the gated call
//	question → a question Item (inProgress) for a tool asking the user
//	           (ask_user free text / choices, or exit_plan_mode's plan)
func (t *translator) interrupt(e turn.TurnInterrupted) []protocol.StreamEvent {
	out := t.closeReasoning()
	out = append(out, t.closeText()...)

	// Snapshot every open tool item before drainTools clears t.tools.
	// Internal-interrupt tools (tools that call hitl.Interrupt inside
	// their Call rather than going through the approval gate) have
	// already been through OnToolCallStart (observedTool.Call fires
	// Start before inner.Call). drainTools closes their items as
	// incomplete — but on resume the tool re-fires and would create a
	// new item without reuse. The snapshot is recorded on the pending
	// interrupt as [interrupts.Pending.DrainedTools] (backend-private —
	// never on the wire payload) so resumeBindingFrom can register the
	// (name, args, itemID) mapping in resume.toolItems.
	//
	// Approval-gate tools exit earlier (gate returns Interrupt before
	// Start fires) and never populate t.tools, so the snapshot is
	// empty for those — correct, because approvalInterrupt creates a
	// fresh item and keys it directly.
	t.parkDrained = drainedToolsFrom(t.tools)

	out = append(out, t.drainTools()...)
	// Close any tool item still open when the turn parks (defensive: the
	// gated call itself paused before item.started, but a sibling tool could
	// be mid-flight) so no started item is left without a terminal (§5.2).

	wire := make([]protocol.Interrupt, 0, len(e.Interrupts))
	for _, in := range e.Interrupts {
		var ev protocol.StreamEvent
		var entry protocol.Interrupt
		switch in.Kind {
		case "approval":
			ev, entry = t.approvalInterrupt(in)
		default: // question — ask_user / exit_plan_mode (both QuestionPrompt)
			ev, entry = t.askUserInterrupt(in)
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
func (t *translator) approvalInterrupt(in turn.Interrupt) (protocol.StreamEvent, protocol.Interrupt) {
	p, _ := in.Payload.(turn.ApprovalPrompt)
	id := t.nextItemID()
	// The gated tool as a full ToolInvocation (arguments parsed, no result
	// yet). The approval Interrupt's payload reuses it (API.md §4.8:
	// ApprovalPayload.tool), so the client reads payload.tool directly
	// instead of guessing where the command / args live.
	inv := t.newToolInvocation(p.ToolName, p.Arguments, "")
	ev := protocol.StreamEvent{
		Type: protocol.StreamItemStarted,
		Item: &protocol.Item{
			ID:          id,
			RunID:       t.runID,
			Status:      protocol.ItemStatusRunning,
			Type:        protocol.ItemTypeToolCall,
			CreatedAt:   time.Now().UTC(),
			Tool:        inv,
			SafetyClass: protocol.SafetyClass(p.SafetyClass),
		},
	}
	// payload carries the self-contained tool (API.md §4.8 ApprovalPayload.tool)
	// plus the gated call's risk + a one-line reason — so the approval card
	// shows them directly, without joining tools.list.
	payload := map[string]any{"tool": inv}
	if p.Risk != "" {
		payload["risk"] = p.Risk
	}
	if p.Reason != "" {
		payload["reason"] = p.Reason
	}
	entry := protocol.Interrupt{
		ItemID: id,
		Type:   protocol.InterruptApproval,
		// payload.tool is the self-contained ApprovalPayload (API.md §4.8): the
		// domain-neutral ToolInvocation always carries name + arguments, so the
		// server re-binds the re-fired approved tool to THIS proposal item
		// across the resume boundary straight off payload.tool (resumeKey on
		// name + canonical arguments) — no backend-internal `_resume` tuple.
		Payload: payload,
	}
	return ev, entry
}

// askUserInterrupt renders a tool's structured question (ask_user or
// exit_plan_mode, both [interrupts.QuestionPrompt]) as an inProgress question
// Item: one wire QuestionField per question — a free-text field, or a
// multiple-choice field when the question carries options (exit_plan_mode
// presents its plan as an Approve / alternatives / Reject choice this way).
// The client answers via runs.resume with an "answer" response keyed by each
// field's name ([interrupts.QuestionFieldName]).
func (t *translator) askUserInterrupt(in turn.Interrupt) (protocol.StreamEvent, protocol.Interrupt) {
	q, _ := in.Payload.(interrupts.QuestionPrompt)
	id := t.nextItemID()
	fields := make([]protocol.QuestionField, len(q.Questions))
	for i, qq := range q.Questions {
		f := protocol.QuestionField{
			Name:     interrupts.QuestionFieldName(i),
			Label:    qq.Question,
			Header:   qq.Header,
			Required: true,
			Type:     protocol.QuestionFieldText,
		}
		if len(qq.Options) > 0 {
			f.Type = protocol.QuestionFieldChoice
			f.Multiple = qq.MultiSelect
			f.Options = make([]protocol.QuestionOption, len(qq.Options))
			for j, o := range qq.Options {
				f.Options[j] = protocol.QuestionOption{Label: o.Label, Description: o.Description}
			}
		}
		fields[i] = f
	}
	// Prompt carries the lone question when there's just one (nicer single-field
	// rendering); with several, each field's Label carries its own question.
	prompt := ""
	if len(q.Questions) == 1 {
		prompt = q.Questions[0].Question
	}
	question := &protocol.Question{
		Prompt: prompt,
		Fields: fields,
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
		Type:    protocol.InterruptQuestion,
		Payload: map[string]any{"question": question},
	}
	return ev, entry
}

// drainedToolsFrom captures every running tool item currently tracked
// in tools as [interrupts.DrainedTool] records. Called before
// [drainTools] so the pending interrupt can carry their
// (name, args, itemID) for resume reuse.
func drainedToolsFrom(tools map[string]*openTool) []interrupts.DrainedTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]interrupts.DrainedTool, 0, len(tools))
	for _, ref := range tools {
		out = append(out, interrupts.DrainedTool{
			ItemID:    ref.id,
			Name:      ref.name,
			Arguments: ref.args,
		})
	}
	return out
}

func (t *translator) drainTools() []protocol.StreamEvent {
	if len(t.tools) == 0 {
		return nil
	}
	out := make([]protocol.StreamEvent, 0, len(t.tools))
	for callID, ref := range t.tools {
		out = append(out, protocol.StreamEvent{
			Type: protocol.StreamItemCompleted,
			Item: &protocol.Item{
				ID:        ref.id,
				RunID:     ref.runID,
				Status:    protocol.ItemStatusIncomplete,
				Type:      protocol.ItemTypeToolCall,
				CreatedAt: ref.createdAt,
				Tool:      t.newToolInvocation(ref.name, ref.args, ""),
			},
		})
		delete(t.tools, callID)
	}
	return out
}
