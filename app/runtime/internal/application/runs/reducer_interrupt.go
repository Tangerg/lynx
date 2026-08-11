package runs

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

func (r *reducer) interrupt(e SegmentInterrupted) (factReduction, error) {
	if err := e.validate(); err != nil {
		return factReduction{}, err
	}
	out, err := r.closeStreaming()
	if err != nil {
		return factReduction{}, err
	}
	parkItems := completedEventItems(nil, out)
	open := r.tools.drain()
	matched, err := matchInterruptTools(open, e.Interrupts)
	if err != nil {
		return factReduction{}, err
	}
	priorDrained := r.resume.remainingDrainedTools()
	r.drained = mergeDrainedTools(priorDrained, drainedToolRefs(open, matched, e.Interrupts))

	approvalItems := make(map[int]transcript.Item, len(matched))
	for _, ref := range open {
		if index, ok := matched[ref]; ok {
			if err := r.closeSuspendedToolAttempt(ref); err != nil {
				return factReduction{}, err
			}
			switch pending := e.Interrupts[index]; pending.Kind {
			case interrupt.Approval:
				item, publishStart, err := r.approvalItem(*pending.Approval, ref)
				if err != nil {
					return factReduction{}, err
				}
				approvalItems[index] = item
				parkItems = append(parkItems, item)
				if publishStart {
					started, err := newToolItemStart(item)
					if err != nil {
						return factReduction{}, err
					}
					out = append(out, ItemStarted{Item: started})
				}
			case interrupt.Question:
				// The question is a separate completed prompt Item, but the tool
				// call that owns it is still suspended inside its handler. Persist
				// that Tool Item as running and carry its identity through the
				// continuation; publishing a completion here would make resume
				// complete the same client block a second time.
				item, err := r.runningToolItem(ref)
				if err != nil {
					return factReduction{}, err
				}
				parkItems = append(parkItems, item)
			}
			continue
		}
		if ref.end != nil {
			completed, err := r.completeTool(ref, *ref.end)
			if err != nil {
				return factReduction{}, err
			}
			out = append(out, completed...)
			parkItems = completedEventItems(parkItems, completed)
			continue
		}
		suspended, err := r.suspendedToolItem(ref)
		if err != nil {
			return factReduction{}, err
		}
		parkItems = append(parkItems, suspended)
	}

	pending := make([]transcript.Interrupt, 0, len(e.Interrupts))
	for index, in := range e.Interrupts {
		var item transcript.Item
		var pendingInterrupt transcript.Interrupt
		switch in.Kind {
		case interrupt.Approval:
			if matchedItem, ok := approvalItems[index]; ok {
				item = matchedItem
				pendingInterrupt = approvalTranscriptInterrupt(item, *in.Approval)
			} else {
				var publishStart bool
				var err error
				item, pendingInterrupt, publishStart, err = r.approvalInterrupt(in)
				if err != nil {
					return factReduction{}, err
				}
				parkItems = append(parkItems, item)
				if publishStart {
					started, err := newToolItemStart(item)
					if err != nil {
						return factReduction{}, err
					}
					out = append(out, ItemStarted{Item: started})
				}
			}
		case interrupt.Question:
			var err error
			item, pendingInterrupt, err = r.questionInterrupt(in)
			if err != nil {
				return factReduction{}, err
			}
			out = append(out, ItemCompleted{Item: item})
			parkItems = append(parkItems, item)
		}
		pending = append(pending, pendingInterrupt)
	}

	r.segmentDuration = e.Duration
	waiting, err := r.runRecord(run.Waiting)
	if err != nil {
		return factReduction{}, err
	}
	return factReduction{
		events:          append(out, SegmentFinished{Run: waiting, Interrupts: pending}),
		parkItems:       parkItems,
		toolInvocations: closedToolInvocationCommits(r.cfg.SegmentID, open),
	}, nil
}

// suspend closes this Run's Segment because another Run in the same tree raised
// the human-input barrier. It carries no direct interrupts, which distinguishes
// a suspended sibling from the Run that owns the barrier. Logical Tool Items stay
// running across the barrier while their segment-scoped attempts end incomplete.
func (r *reducer) suspend(duration time.Duration) (factReduction, error) {
	out, err := r.closeStreaming()
	if err != nil {
		return factReduction{}, err
	}
	parkItems := completedEventItems(nil, out)
	open := r.tools.drain()
	r.drained = mergeDrainedTools(
		r.resume.remainingDrainedTools(),
		drainedToolRefs(open, nil, nil),
	)
	for _, ref := range open {
		if ref.end != nil {
			completed, err := r.completeTool(ref, *ref.end)
			if err != nil {
				return factReduction{}, err
			}
			out = append(out, completed...)
			parkItems = completedEventItems(parkItems, completed)
			continue
		}
		suspended, err := r.suspendedToolItem(ref)
		if err != nil {
			return factReduction{}, err
		}
		parkItems = append(parkItems, suspended)
	}
	r.segmentDuration = duration
	waiting, err := r.runRecord(run.Waiting)
	if err != nil {
		return factReduction{}, err
	}
	return factReduction{
		events:          append(out, SegmentFinished{Run: waiting}),
		parkItems:       parkItems,
		toolInvocations: closedToolInvocationCommits(r.cfg.SegmentID, open),
	}, nil
}

func completedEventItems(items []transcript.Item, events []RunEvent) []transcript.Item {
	for _, event := range events {
		if completed, ok := event.(ItemCompleted); ok {
			items = append(items, completed.Item)
		}
	}
	return items
}

func (r *reducer) approvalInterrupt(in Interrupt) (transcript.Item, transcript.Interrupt, bool, error) {
	if in.Approval == nil {
		return transcript.Item{}, transcript.Interrupt{}, false, nil
	}
	item, publishStart, err := r.approvalItem(*in.Approval, nil)
	if err != nil {
		return transcript.Item{}, transcript.Interrupt{}, false, err
	}
	return item, approvalTranscriptInterrupt(item, *in.Approval), publishStart, nil
}

func (r *reducer) approvalItem(prompt ApprovalPrompt, ref *openTool) (transcript.Item, bool, error) {
	arguments, err := parseToolArguments(prompt.Arguments)
	if err != nil {
		return transcript.Item{}, false, fmt.Errorf("approval tool %q arguments: %w", prompt.ToolName, err)
	}
	var id string
	var startedAt time.Time
	publishStart := false
	if ref != nil {
		id, startedAt = ref.id, ref.occurredAt
	} else {
		identity, reused := r.reuseOrCreateToolItem(prompt.CallID, prompt.ToolName, arguments)
		id, startedAt = identity.id, identity.occurredAt
		publishStart = !reused
		r.removeDrained(id)
	}
	item, err := transcript.NewToolCall(
		r.itemIdentity(id, startedAt),
		*newToolInvocation(prompt.ToolName, arguments, nil),
		prompt.SafetyClass,
	)
	return item, publishStart, err
}

func approvalTranscriptInterrupt(item transcript.Item, prompt ApprovalPrompt) transcript.Interrupt {
	invocation, _ := item.ToolInvocation()
	return transcript.Interrupt{
		ItemID:         item.ID(),
		ItemOccurredAt: item.OccurredAt(),
		RunID:          item.RunID(),
		Kind:           interrupt.Approval,
		Approval: &transcript.Approval{
			Tool: invocation, Risk: prompt.Risk, Reason: prompt.Reason, Rememberable: prompt.Rememberable,
		},
	}
}

// matchInterruptTools binds an executor interrupt back to the open tool call
// that raised it. Approval carries a provider call ID; question-producing tools
// are correlated by their canonical name and arguments because their handler
// creates the interrupt below the execution wrapper that owns that ID.
func matchInterruptTools(open []*openTool, values []Interrupt) (map[*openTool]int, error) {
	matched := make(map[*openTool]int)
	for index, value := range values {
		toolName, rawArguments := value.Tool()
		if toolName == "" {
			continue
		}
		arguments, err := parseToolArguments(rawArguments)
		if err != nil {
			return nil, fmt.Errorf("%s interrupt tool %q arguments: %w", value.Kind, toolName, err)
		}
		callID := ""
		switch {
		case value.Approval != nil:
			callID = value.Approval.CallID
		case value.Question != nil:
			callID = value.Question.CallID
		}
		for _, ref := range open {
			if ref.end != nil {
				continue
			}
			if _, used := matched[ref]; used {
				continue
			}
			if callID != "" {
				if ref.callID != callID {
					continue
				}
			} else if ref.name != toolName || argumentIdentity(ref.arguments) != argumentIdentity(arguments) {
				continue
			}
			matched[ref] = index
			break
		}
	}
	return matched, nil
}

func drainedToolRefs(
	open []*openTool,
	matched map[*openTool]int,
	interrupts []Interrupt,
) []DrainedTool {
	var drained []DrainedTool
	for _, ref := range open {
		matchedIndex, matchedInterrupt := matched[ref]
		activeApproval := matchedInterrupt && interrupts[matchedIndex].Kind == interrupt.Approval
		if ref.end == nil && !activeApproval {
			drained = append(drained, DrainedTool{
				ItemID: ref.id, ItemOccurredAt: ref.occurredAt,
				CallID: ref.callID, SourceCallID: ref.sourceCallID,
				Name: ref.name, Arguments: ref.arguments.Canonical(),
			})
		}
	}
	return drained
}

func mergeDrainedTools(groups ...[]DrainedTool) []DrainedTool {
	var merged []DrainedTool
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
	r.drained = slices.DeleteFunc(r.drained, func(tool DrainedTool) bool {
		return tool.ItemID == itemID
	})
}

func (r *reducer) incompleteToolItem(ref *openTool) (ItemCompleted, error) {
	if err := r.closeSuspendedToolAttempt(ref); err != nil {
		return ItemCompleted{}, err
	}
	item, err := r.runningToolItem(ref)
	if err != nil {
		return ItemCompleted{}, err
	}
	item, err = item.AbandonToolCall(nil, ref.finishedAt)
	if err != nil {
		return ItemCompleted{}, err
	}
	return ItemCompleted{Item: item}, nil
}

func (r *reducer) suspendedToolItem(ref *openTool) (transcript.Item, error) {
	if err := r.closeSuspendedToolAttempt(ref); err != nil {
		return transcript.Item{}, err
	}
	return r.runningToolItem(ref)
}

func (r *reducer) closeSuspendedToolAttempt(ref *openTool) error {
	finishedAt := r.now()
	if finishedAt.Before(ref.attemptStartedAt) {
		return fmt.Errorf("tool call %q finish time precedes start time", ref.callID)
	}
	ref.finishedAt = finishedAt
	return nil
}

func (r *reducer) questionInterrupt(in Interrupt) (transcript.Item, transcript.Interrupt, error) {
	if in.Question == nil {
		return transcript.Item{}, transcript.Interrupt{}, nil
	}
	question := questionFromPrompt(*in.Question)
	id := r.nextItemID()
	item, err := transcript.NewQuestion(r.itemIdentity(id, r.now()), question)
	if err != nil {
		return transcript.Item{}, transcript.Interrupt{}, err
	}
	return item, transcript.Interrupt{
		ItemID: id, ItemOccurredAt: item.OccurredAt(),
		RunID: r.cfg.RunID, Kind: interrupt.Question, Question: &question,
	}, nil
}

func questionFromPrompt(prompt QuestionPrompt) transcript.Question {
	fields := make([]transcript.QuestionField, len(prompt.Fields))
	for i, question := range prompt.Fields {
		field := transcript.QuestionField{
			Prompt: question.Prompt, Header: question.Header, Kind: transcript.QuestionText,
		}
		if len(question.Options) > 0 {
			field.Kind = transcript.QuestionChoice
			field.Multiple = question.Multiple
			field.AllowCustom = question.AllowCustom
			field.Options = make([]transcript.QuestionOption, len(question.Options))
			for j, option := range question.Options {
				field.Options[j] = transcript.QuestionOption{Label: option.Label, Description: option.Description}
			}
		}
		fields[i] = field
	}
	return transcript.Question{Fields: fields}
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
	slices.SortFunc(ordered, func(a, b *openTool) int {
		aAttributed := a.modelCallSequence > 0
		bAttributed := b.modelCallSequence > 0
		if aAttributed != bAttributed {
			if aAttributed {
				return 1
			}
			return -1
		}
		if aAttributed {
			if byModelCall := cmp.Compare(a.modelCallSequence, b.modelCallSequence); byModelCall != 0 {
				return byModelCall
			}
			return cmp.Compare(a.toolCallIndex, b.toolCallIndex)
		}
		return cmp.Compare(a.arrivalOrder, b.arrivalOrder)
	})
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
		incomplete, err := r.incompleteToolItem(ref)
		if err != nil {
			return nil, err
		}
		out = append(out, incomplete)
	}
	return out, nil
}
