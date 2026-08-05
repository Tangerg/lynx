package runs

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

type resumeBinding struct {
	callItems map[string]resumableItem
	toolItems map[string]resumableItem
	byName    map[string]resumableItem
	questions []resumedQuestion
	drained   []interrupts.DrainedTool
	committed map[string]interrupts.CommittedTool
	consumed  map[string]struct{}
	err       error
}

type resumedQuestion struct {
	itemID     string
	occurredAt time.Time
	question   *transcript.Question
}

type resumableItem struct {
	id         string
	occurredAt time.Time
}

func resumeBindingFrom(continuation treeContinuation, runID string) *resumeBinding {
	calls := map[string]resumableItem{}
	items := map[string]resumableItem{}
	byName := map[string]resumableItem{}
	addItem := func(callID, name, arguments, itemID string, occurredAt time.Time) {
		identity := resumableItem{id: itemID, occurredAt: occurredAt}
		if callID != "" {
			calls[callID] = identity
		}
		items[resumeKey(name, arguments)] = identity
		if _, duplicate := byName[name]; duplicate {
			byName[name] = resumableItem{}
		} else {
			byName[name] = identity
		}
	}

	var questions []resumedQuestion
	for _, in := range continuation.interrupts {
		if in.RunID != runID || in.ItemID == "" {
			continue
		}
		switch in.Kind {
		case execution.ApprovalInterrupt:
			if in.Approval != nil && in.Approval.Tool.Name != "" {
				addItem("", in.Approval.Tool.Name, argumentIdentity(in.Approval.Tool.Arguments), in.ItemID, in.ItemOccurredAt)
			}
		case execution.QuestionInterrupt:
			questions = append(questions, resumedQuestion{
				itemID: in.ItemID, occurredAt: in.ItemOccurredAt, question: in.Question,
			})
		}
	}
	var drained []interrupts.DrainedTool
	committed := make(map[string]interrupts.CommittedTool)
	if member, ok := continuation.forRun(runID); ok {
		drained = member.DrainedTools
		for _, tool := range drained {
			if tool.Name != "" && tool.ItemID != "" {
				arguments, err := parseToolArguments(tool.Arguments)
				if err != nil {
					return &resumeBinding{err: fmt.Errorf("resume drained tool %q arguments: %w", tool.Name, err)}
				}
				addItem(tool.CallID, tool.Name, argumentIdentity(arguments), tool.ItemID, tool.ItemOccurredAt)
			}
		}
		for _, tool := range member.CommittedTools {
			arguments, err := parseToolArguments(tool.Arguments)
			if err != nil {
				return &resumeBinding{err: fmt.Errorf("resume committed tool %q arguments: %w", tool.Name, err)}
			}
			tool.Arguments = argumentIdentity(arguments)
			committed[tool.CallID] = tool
		}
	}
	if len(calls) == 0 && len(items) == 0 && len(questions) == 0 && len(committed) == 0 {
		return nil
	}
	return &resumeBinding{
		callItems: calls, toolItems: items, byName: byName, questions: questions,
		drained: slices.Clone(drained), committed: committed, consumed: make(map[string]struct{}),
	}
}

func resumeKey(toolName, arguments string) string { return toolName + "\x00" + arguments }

func argumentIdentity(arguments tool.Arguments) string { return arguments.Canonical() }

func (r *reducer) reuseOrCreateToolItem(callID, toolName string, arguments tool.Arguments) resumableItem {
	if r.resume != nil {
		if item, ok := r.resume.callItems[callID]; callID != "" && ok {
			r.resume.consumeToolItem(item.id)
			return item
		}
		key := resumeKey(toolName, argumentIdentity(arguments))
		if item, ok := r.resume.toolItems[key]; ok {
			r.resume.consumeToolItem(item.id)
			return item
		}
		if item, ok := r.resume.byName[toolName]; ok && item.id != "" {
			r.resume.consumeToolItem(item.id)
			return item
		}
	}
	return resumableItem{id: r.nextItemID(), occurredAt: r.now()}
}

func (b *resumeBinding) consumeToolItem(id string) {
	if id == "" {
		return
	}
	b.consumed[id] = struct{}{}
	maps.DeleteFunc(b.callItems, func(_ string, candidate resumableItem) bool { return candidate.id == id })
	maps.DeleteFunc(b.toolItems, func(_ string, candidate resumableItem) bool { return candidate.id == id })
	maps.DeleteFunc(b.byName, func(_ string, candidate resumableItem) bool { return candidate.id == id })
}

func (b *resumeBinding) remainingDrainedTools() []interrupts.DrainedTool {
	if b == nil || len(b.drained) == 0 {
		return nil
	}
	out := make([]interrupts.DrainedTool, 0, len(b.drained))
	for _, tool := range b.drained {
		if _, consumed := b.consumed[tool.ItemID]; !consumed {
			out = append(out, tool)
		}
	}
	return out
}

func (b *resumeBinding) rejectCommittedToolStart(
	callID string,
	toolName string,
	arguments tool.Arguments,
) error {
	if b == nil {
		return nil
	}
	committed, exists := b.committed[callID]
	if !exists {
		return nil
	}
	if committed.Name != toolName || committed.Arguments != argumentIdentity(arguments) {
		return fmt.Errorf(
			"committed tool call %q replayed as %q/%s, want %q/%s",
			callID,
			toolName,
			argumentIdentity(arguments),
			committed.Name,
			committed.Arguments,
		)
	}
	return fmt.Errorf("committed tool call %q was executed again", callID)
}

func (b *resumeBinding) consumeCommittedTool(event ToolCallFinished) (bool, error) {
	if b == nil {
		return false, nil
	}
	committed, exists := b.committed[event.CallID]
	if !exists {
		return false, nil
	}
	if event.Problem == nil {
		return true, fmt.Errorf("committed tool call %q published a successful result", event.CallID)
	}
	if event.Arguments != "" {
		arguments, err := parseToolArguments(event.Arguments)
		if err != nil {
			return true, fmt.Errorf("committed tool call %q arguments: %w", event.CallID, err)
		}
		if argumentIdentity(arguments) != committed.Arguments {
			return true, fmt.Errorf(
				"committed tool call %q arguments changed from %s to %s",
				event.CallID,
				committed.Arguments,
				argumentIdentity(arguments),
			)
		}
	}
	delete(b.committed, event.CallID)
	return true, nil
}

func (b *resumeBinding) remainingCommittedTools() []interrupts.CommittedTool {
	if b == nil || len(b.committed) == 0 {
		return nil
	}
	out := make([]interrupts.CommittedTool, 0, len(b.committed))
	for _, committed := range b.committed {
		out = append(out, committed)
	}
	slices.SortFunc(out, func(left, right interrupts.CommittedTool) int {
		return strings.Compare(left.CallID, right.CallID)
	})
	return out
}

func (r *reducer) resumeQuestionCompletions() []RunEvent {
	if r.resume == nil || len(r.resume.questions) == 0 {
		return nil
	}
	out := make([]RunEvent, 0, len(r.resume.questions))
	for _, question := range r.resume.questions {
		out = append(out, ItemCompleted{Item: transcript.Item{
			ID: question.itemID, RunID: r.cfg.RunID, Status: transcript.ItemCompleted,
			Kind: transcript.QuestionItem, OccurredAt: question.occurredAt, Question: question.question,
		}})
	}
	r.resume.questions = nil
	return out
}
