package runs

import (
	"cmp"
	"fmt"
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
	open := r.tools.drain()
	matched, err := matchApprovalTools(open, e.Interrupts)
	if err != nil {
		return nil, err
	}
	priorDrained := r.resume.remainingDrainedTools()
	r.drained = mergeDrainedTools(priorDrained, drainedToolRefs(open, matched))

	approvalItems := make(map[int]transcript.Item, len(matched))
	for _, ref := range open {
		if index, ok := matched[ref]; ok {
			item, err := r.approvalItem(*e.Interrupts[index].Approval, ref)
			if err != nil {
				return nil, err
			}
			approvalItems[index] = item
			out = append(out, ItemStarted{Item: item})
			continue
		}
		if ref.end != nil {
			completed, err := r.completeTool(ref, *ref.end)
			if err != nil {
				return nil, err
			}
			out = append(out, completed...)
			continue
		}
		out = append(out, incompleteToolItem(r.cfg.RunID, ref))
	}

	pending := make([]transcript.Interrupt, 0, len(e.Interrupts))
	for index, in := range e.Interrupts {
		var item transcript.Item
		var interrupt transcript.Interrupt
		switch in.Kind {
		case ApprovalInterruptKind:
			if matchedItem, ok := approvalItems[index]; ok {
				item = matchedItem
				interrupt = approvalTranscriptInterrupt(item.ID, *in.Approval, *item.Tool)
			} else {
				var err error
				item, interrupt, err = r.approvalInterrupt(in)
				if err != nil {
					return nil, err
				}
				out = append(out, ItemStarted{Item: item})
			}
		case QuestionInterruptKind:
			item, interrupt = r.questionInterrupt(in)
			out = append(out, ItemStarted{Item: item})
		}
		pending = append(pending, interrupt)
	}

	run := r.runRecord(execution.Interrupted)
	run.Interrupts = pending
	return append(out, SegmentFinished{Run: run}), nil
}

func (r *reducer) approvalInterrupt(in Interrupt) (transcript.Item, transcript.Interrupt, error) {
	if in.Approval == nil {
		return transcript.Item{}, transcript.Interrupt{}, nil
	}
	item, err := r.approvalItem(*in.Approval, nil)
	if err != nil {
		return transcript.Item{}, transcript.Interrupt{}, err
	}
	return item, approvalTranscriptInterrupt(item.ID, *in.Approval, *item.Tool), nil
}

func (r *reducer) approvalItem(prompt ApprovalPrompt, ref *openTool) (transcript.Item, error) {
	arguments, err := parseToolArguments(prompt.Arguments)
	if err != nil {
		return transcript.Item{}, fmt.Errorf("approval tool %q arguments: %w", prompt.ToolName, err)
	}
	id, createdAt := "", r.now()
	if ref != nil {
		id, createdAt = ref.id, ref.createdAt
	} else {
		id = r.reuseOrNextItemID(prompt.CallID, prompt.ToolName, arguments)
		r.removeDrained(id)
	}
	return transcript.Item{
		ID: id, RunID: r.cfg.RunID, Status: transcript.ItemRunning,
		Kind: transcript.ToolCall, CreatedAt: createdAt,
		Tool:        newToolInvocation(prompt.ToolName, arguments, nil),
		SafetyClass: prompt.SafetyClass,
	}, nil
}

func approvalTranscriptInterrupt(itemID string, prompt ApprovalPrompt, tool transcript.ToolInvocation) transcript.Interrupt {
	return transcript.Interrupt{
		ItemID: itemID,
		Kind:   transcript.ApprovalInterrupt,
		Approval: &transcript.Approval{
			Tool: tool, Risk: prompt.Risk, Reason: prompt.Reason, Rememberable: prompt.Rememberable,
		},
	}
}

func matchApprovalTools(open []*openTool, values []Interrupt) (map[*openTool]int, error) {
	matched := make(map[*openTool]int)
	for index, value := range values {
		if value.Kind != ApprovalInterruptKind || value.Approval == nil {
			continue
		}
		prompt := value.Approval
		arguments, err := parseToolArguments(prompt.Arguments)
		if err != nil {
			return nil, fmt.Errorf("approval tool %q arguments: %w", prompt.ToolName, err)
		}
		for _, ref := range open {
			if ref.end != nil {
				continue
			}
			if _, used := matched[ref]; used {
				continue
			}
			if prompt.CallID != "" {
				if ref.callID != prompt.CallID {
					continue
				}
			} else if ref.name != prompt.ToolName || argumentIdentity(ref.arguments) != argumentIdentity(arguments) {
				continue
			}
			matched[ref] = index
			break
		}
	}
	return matched, nil
}

func drainedToolRefs(open []*openTool, matched map[*openTool]int) []interrupts.DrainedTool {
	var drained []interrupts.DrainedTool
	for _, ref := range open {
		_, activeApproval := matched[ref]
		if ref.end == nil && !activeApproval {
			drained = append(drained, interrupts.DrainedTool{
				ItemID: ref.id, CallID: ref.callID, Name: ref.name, Arguments: ref.arguments.Canonical(),
			})
		}
	}
	return drained
}

func mergeDrainedTools(groups ...[]interrupts.DrainedTool) []interrupts.DrainedTool {
	var merged []interrupts.DrainedTool
	seen := make(map[string]struct{})
	for _, group := range groups {
		for _, tool := range group {
			if _, duplicate := seen[tool.ItemID]; duplicate {
				continue
			}
			seen[tool.ItemID] = struct{}{}
			merged = append(merged, tool)
		}
	}
	return merged
}

func (r *reducer) removeDrained(itemID string) {
	r.drained = slices.DeleteFunc(r.drained, func(tool interrupts.DrainedTool) bool {
		return tool.ItemID == itemID
	})
}

func incompleteToolItem(runID string, ref *openTool) ItemCompleted {
	return ItemCompleted{Item: transcript.Item{
		ID: ref.id, RunID: runID, Status: transcript.ItemIncomplete,
		Kind: transcript.ToolCall, CreatedAt: ref.createdAt,
		Tool:        newToolInvocation(ref.name, ref.arguments, nil),
		SafetyClass: ref.safetyClass,
	}}
}

func (r *reducer) questionInterrupt(in Interrupt) (transcript.Item, transcript.Interrupt) {
	if in.Question == nil {
		return transcript.Item{}, transcript.Interrupt{}
	}
	question := questionFromPrompt(*in.Question)
	id := r.nextItemID()
	item := transcript.Item{
		ID: id, RunID: r.cfg.RunID, Status: transcript.ItemRunning,
		Kind: transcript.QuestionItem, CreatedAt: r.now(), Question: &question,
	}
	return item, transcript.Interrupt{
		ItemID: id, Kind: transcript.QuestionInterrupt, Question: &question,
	}
}

func questionFromPrompt(prompt QuestionPrompt) transcript.Question {
	fields := make([]transcript.QuestionField, len(prompt.Questions))
	for i, question := range prompt.Questions {
		field := transcript.QuestionField{
			Name: QuestionFieldID(i), Label: question.Question,
			Header: question.Header, Required: true, Kind: transcript.QuestionText,
		}
		if len(question.Options) > 0 {
			field.Kind = transcript.QuestionChoice
			field.Multiple = question.MultiSelect
			field.Options = make([]transcript.QuestionOption, len(question.Options))
			for j, option := range question.Options {
				field.Options[j] = transcript.QuestionOption{Label: option.Label, Description: option.Description}
			}
		}
		fields[i] = field
	}
	label := ""
	if len(prompt.Questions) == 1 {
		label = prompt.Questions[0].Question
	}
	return transcript.Question{Prompt: label, Fields: fields}
}

type openTools map[string]*openTool

func (tools openTools) add(tool *openTool) {
	tools[tool.callID] = tool
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

func (r *reducer) drainTools() ([]RunEvent, error) {
	tools := r.tools.drain()
	if len(tools) == 0 {
		return nil, nil
	}
	var out []RunEvent
	for _, ref := range tools {
		if ref.end != nil {
			completed, err := r.completeTool(ref, *ref.end)
			if err != nil {
				return nil, err
			}
			out = append(out, completed...)
			continue
		}
		out = append(out, incompleteToolItem(r.cfg.RunID, ref))
	}
	return out, nil
}
