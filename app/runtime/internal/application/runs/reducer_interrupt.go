package runs

import (
	"cmp"
	"maps"
	"slices"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

func (r *reducer) interrupt(e TurnInterrupted) ([]RunEvent, error) {
	if err := e.validate(); err != nil {
		return nil, err
	}
	out := r.closeStreaming()
	r.drained = r.tools.snapshot()
	out = append(out, r.drainTools()...)

	pending := make([]transcript.Interrupt, 0, len(e.Interrupts))
	for _, in := range e.Interrupts {
		var item Item
		var interrupt transcript.Interrupt
		switch in.Kind {
		case ApprovalInterruptKind:
			item, interrupt = r.approvalInterrupt(in)
		case QuestionInterruptKind:
			item, interrupt = r.questionInterrupt(in)
		}
		out = append(out, ItemStarted{Item: item})
		pending = append(pending, interrupt)
	}

	run := r.runRecord(execution.Interrupted)
	run.Interrupts = pending
	return append(out, SegmentFinished{Run: run}), nil
}

func (r *reducer) approvalInterrupt(in Interrupt) (Item, transcript.Interrupt) {
	if in.Approval == nil {
		return Item{}, transcript.Interrupt{}
	}
	p := in.Approval
	id := r.nextItemID()
	tool := newToolInvocation(p.ToolName, p.Arguments, nil)
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

type openTools map[string]*openTool

func (tools openTools) add(tool *openTool) {
	tools[tool.callID] = tool
}

func (tools openTools) take(callID string) (*openTool, bool) {
	tool, ok := tools[callID]
	if ok {
		delete(tools, callID)
	}
	return tool, ok
}

func (tools openTools) snapshot() []interrupts.DrainedTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]interrupts.DrainedTool, 0, len(tools))
	for _, ref := range tools.ordered() {
		out = append(out, interrupts.DrainedTool{ItemID: ref.id, Name: ref.name, Arguments: ref.args})
	}
	return out
}

func (tools openTools) drain() []*openTool {
	ordered := tools.ordered()
	clear(tools)
	return ordered
}

func (tools openTools) ordered() []*openTool {
	ordered := slices.Collect(maps.Values(tools))
	slices.SortFunc(ordered, func(a, b *openTool) int { return cmp.Compare(a.order, b.order) })
	return ordered
}

func (r *reducer) drainTools() []RunEvent {
	tools := r.tools.drain()
	if len(tools) == 0 {
		return nil
	}
	out := make([]RunEvent, 0, len(tools))
	for _, ref := range tools {
		out = append(out, ItemCompleted{Item: Item{
			ID: ref.id, RunID: r.cfg.RunID, Status: ItemIncomplete,
			Kind: ToolCall, CreatedAt: ref.createdAt,
			Tool:        newToolInvocation(ref.name, ref.args, nil),
			SafetyClass: ref.safetyClass,
		}})
	}
	return out
}
