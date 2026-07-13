package runs

import (
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

func (r *reducer) interrupt(e TurnInterrupted) []RunEvent {
	out := r.closeStreaming()
	r.drained = drainedToolsFrom(r.tools)
	out = append(out, r.drainTools()...)

	pending := make([]transcript.Interrupt, 0, len(e.Interrupts))
	for _, in := range e.Interrupts {
		var item Item
		var interrupt transcript.Interrupt
		switch in.Kind {
		case "approval":
			item, interrupt = r.approvalInterrupt(in)
		default:
			item, interrupt = r.questionInterrupt(in)
		}
		if item.ID == "" {
			continue
		}
		out = append(out, ItemStarted{Item: item})
		pending = append(pending, interrupt)
	}

	run := r.runRecord(execution.Interrupted)
	run.Interrupts = pending
	return append(out, SegmentFinished{Run: run})
}

func (r *reducer) approvalInterrupt(in Interrupt) (Item, transcript.Interrupt) {
	if in.Approval == nil {
		return Item{}, transcript.Interrupt{}
	}
	p := in.Approval
	id := r.nextItemID()
	tool := r.newToolInvocation(p.ToolName, p.Arguments, "")
	item := Item{
		ID: id, RunID: r.cfg.RunID, Status: ItemRunning,
		Kind: ToolCall, CreatedAt: r.now(), Tool: tool,
		SafetyClass: p.SafetyClass,
	}
	return item, transcript.Interrupt{
		ItemID: id,
		Kind:   transcript.ApprovalInterrupt,
		Approval: &transcript.Approval{
			Tool: *tool, Risk: p.Risk, Reason: p.Reason,
		},
	}
}

func (r *reducer) questionInterrupt(in Interrupt) (Item, transcript.Interrupt) {
	if in.Question == nil {
		return Item{}, transcript.Interrupt{}
	}
	question := questionFromPrompt(*in.Question)
	id := r.nextItemID()
	item := Item{
		ID: id, RunID: r.cfg.RunID, Status: ItemRunning,
		Kind: QuestionItem, CreatedAt: r.now(), Question: &question,
	}
	return item, transcript.Interrupt{
		ItemID: id, Kind: transcript.QuestionInterrupt, Question: &question,
	}
}

func questionFromPrompt(prompt interrupts.QuestionPrompt) Question {
	fields := make([]QuestionField, len(prompt.Questions))
	for i, question := range prompt.Questions {
		field := QuestionField{
			Name: interrupts.QuestionFieldName(i), Label: question.Question,
			Header: question.Header, Required: true, Kind: QuestionText,
		}
		if len(question.Options) > 0 {
			field.Kind = QuestionChoice
			field.Multiple = question.MultiSelect
			field.Options = make([]QuestionOption, len(question.Options))
			for j, option := range question.Options {
				field.Options[j] = QuestionOption{Label: option.Label, Description: option.Description}
			}
		}
		fields[i] = field
	}
	label := ""
	if len(prompt.Questions) == 1 {
		label = prompt.Questions[0].Question
	}
	return Question{Prompt: label, Fields: fields}
}

func drainedToolsFrom(tools map[string]*openTool) []interrupts.DrainedTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]interrupts.DrainedTool, 0, len(tools))
	for _, ref := range tools {
		out = append(out, interrupts.DrainedTool{ItemID: ref.id, Name: ref.name, Arguments: ref.args})
	}
	return out
}

func (r *reducer) drainTools() []RunEvent {
	if len(r.tools) == 0 {
		return nil
	}
	out := make([]RunEvent, 0, len(r.tools))
	for callID, ref := range r.tools {
		out = append(out, ItemCompleted{Item: Item{
			ID: ref.id, RunID: r.cfg.RunID, Status: ItemIncomplete,
			Kind: ToolCall, CreatedAt: ref.createdAt,
			Tool:        r.newToolInvocation(ref.name, ref.args, ""),
			SafetyClass: ref.safetyClass,
		}})
		delete(r.tools, callID)
	}
	return out
}
