package runs

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

type resumeBinding struct {
	callItems map[string]resumableItem
	toolItems map[string]resumableItem
	byName    map[string]resumableItem
	drained   []DrainedTool
	committed map[string]CommittedTool
	consumed  map[string]struct{}
	err       error
}

type resumableItem struct {
	id               string
	occurredAt       time.Time
	approvalDecision approval.Decision
}

func resumeBindingFrom(continuation treeContinuation, runID string) *resumeBinding {
	builder := newResumeBindingBuilder(continuation.approvalResolutions)
	builder.addInterrupts(continuation.interrupts, runID)
	if member, found := continuation.forRun(runID); found {
		if err := builder.addTools(member); err != nil {
			return &resumeBinding{err: err}
		}
	}
	return builder.build()
}

type resumeBindingBuilder struct {
	binding             resumeBinding
	approvalResolutions map[string]ToolApprovalResolution
}

func newResumeBindingBuilder(resolutions map[string]ToolApprovalResolution) *resumeBindingBuilder {
	return &resumeBindingBuilder{approvalResolutions: resolutions, binding: resumeBinding{
		callItems: make(map[string]resumableItem),
		toolItems: make(map[string]resumableItem),
		byName:    make(map[string]resumableItem),
		committed: make(map[string]CommittedTool),
		consumed:  make(map[string]struct{}),
	}}
}

func (builder *resumeBindingBuilder) addItem(
	callID string,
	name string,
	arguments string,
	itemID string,
	occurredAt time.Time,
	decision approval.Decision,
) {
	identity := resumableItem{id: itemID, occurredAt: occurredAt, approvalDecision: decision}
	if callID != "" {
		builder.binding.callItems[callID] = identity
	}
	builder.binding.toolItems[resumeKey(name, arguments)] = identity
	if _, duplicate := builder.binding.byName[name]; duplicate {
		builder.binding.byName[name] = resumableItem{}
	} else {
		builder.binding.byName[name] = identity
	}
}

func (builder *resumeBindingBuilder) addInterrupts(interrupts []transcript.Interrupt, runID string) {
	for _, pending := range interrupts {
		if pending.RunID != runID || pending.ItemID == "" {
			continue
		}
		switch pending.Kind {
		case interrupt.Approval:
			if pending.Approval != nil && pending.Approval.Tool.Name != "" {
				resolution := builder.approvalResolutions[pending.ItemID]
				builder.addItem(
					resolution.CallID,
					pending.Approval.Tool.Name,
					argumentIdentity(pending.Approval.Tool.Arguments),
					pending.ItemID,
					pending.ItemOccurredAt,
					resolution.Decision,
				)
			}
		case interrupt.Question:
			// Question Items are complete prompt facts when the tree parks. Pending
			// owns whether an answer is still outstanding, so resume has no Item
			// lifecycle to settle.
		}
	}
}

func (builder *resumeBindingBuilder) addTools(member Continuation) error {
	builder.binding.drained = slices.Clone(member.DrainedTools)
	for _, drained := range member.DrainedTools {
		if drained.Name == "" || drained.ItemID == "" {
			continue
		}
		arguments, err := parseToolArguments(drained.Arguments)
		if err != nil {
			return fmt.Errorf("resume drained tool %q arguments: %w", drained.Name, err)
		}
		builder.addItem(
			drained.CallID,
			drained.Name,
			argumentIdentity(arguments),
			drained.ItemID,
			drained.ItemOccurredAt,
			"",
		)
	}
	for _, committed := range member.CommittedTools {
		arguments, err := parseToolArguments(committed.Arguments)
		if err != nil {
			return fmt.Errorf("resume committed tool %q arguments: %w", committed.Name, err)
		}
		committed.Arguments = argumentIdentity(arguments)
		builder.binding.committed[committed.CallID] = committed
	}
	return nil
}

func (builder *resumeBindingBuilder) build() *resumeBinding {
	binding := &builder.binding
	if len(binding.callItems) == 0 && len(binding.toolItems) == 0 &&
		len(binding.committed) == 0 {
		return nil
	}
	return binding
}

func resumeKey(toolName, arguments string) string { return toolName + "\x00" + arguments }

func argumentIdentity(arguments tool.Arguments) string { return arguments.Canonical() }

func (r *reducer) reuseOrCreateToolItem(callID, toolName string, arguments tool.Arguments) (resumableItem, bool) {
	if r.resume != nil {
		if item, ok := r.resume.callItems[callID]; callID != "" && ok {
			r.resume.consumeToolItem(item.id)
			return item, true
		}
		key := resumeKey(toolName, argumentIdentity(arguments))
		if item, ok := r.resume.toolItems[key]; ok {
			r.resume.consumeToolItem(item.id)
			return item, true
		}
		if item, ok := r.resume.byName[toolName]; ok && item.id != "" {
			r.resume.consumeToolItem(item.id)
			return item, true
		}
	}
	return resumableItem{id: r.nextItemID(), occurredAt: r.now()}, false
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

func (b *resumeBinding) approvalDecision(itemID string) approval.Decision {
	if b == nil || itemID == "" {
		return ""
	}
	for _, items := range []map[string]resumableItem{b.callItems, b.toolItems, b.byName} {
		for _, item := range items {
			if item.id == itemID {
				return item.approvalDecision
			}
		}
	}
	return ""
}

func (b *resumeBinding) remainingDrainedTools() []DrainedTool {
	if b == nil || len(b.drained) == 0 {
		return nil
	}
	out := make([]DrainedTool, 0, len(b.drained))
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
	if event.Failure == nil {
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

func (b *resumeBinding) remainingCommittedTools() []CommittedTool {
	if b == nil || len(b.committed) == 0 {
		return nil
	}
	out := make([]CommittedTool, 0, len(b.committed))
	for _, committed := range b.committed {
		out = append(out, committed)
	}
	slices.SortFunc(out, func(left, right CommittedTool) int {
		return strings.Compare(left.CallID, right.CallID)
	})
	return out
}
