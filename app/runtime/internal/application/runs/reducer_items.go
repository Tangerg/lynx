package runs

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
)

func itemPair(build func(transcript.ItemStatus) transcript.Item) []RunEvent {
	return []RunEvent{
		ItemStarted{Item: build(transcript.ItemRunning)},
		ItemCompleted{Item: build(transcript.ItemCompleted)},
	}
}

func (r *reducer) appendText(text string) []RunEvent {
	var out []RunEvent
	if r.text == nil {
		r.text = &openText{id: r.nextItemID(), createdAt: r.now()}
		out = append(out, ItemStarted{Item: transcript.Item{
			ID: r.text.id, RunID: r.cfg.RunID, Status: transcript.ItemRunning,
			Kind: transcript.AgentMessage, CreatedAt: r.text.createdAt,
		}})
	}
	r.text.buf.WriteString(text)
	index := 0
	return append(out, ItemChanged{
		ItemID: r.text.id,
		Delta:  ItemDelta{Kind: ContentDelta, Index: &index, Text: text},
	})
}

func (r *reducer) appendReasoning(text string) []RunEvent {
	var out []RunEvent
	if r.reasoning == nil {
		r.reasoning = &openText{id: r.nextItemID(), createdAt: r.now()}
		out = append(out, ItemStarted{Item: transcript.Item{
			ID: r.reasoning.id, RunID: r.cfg.RunID, Status: transcript.ItemRunning,
			Kind: transcript.Reasoning, CreatedAt: r.reasoning.createdAt,
		}})
	}
	r.reasoning.buf.WriteString(text)
	return append(out, ItemChanged{
		ItemID: r.reasoning.id,
		Delta:  ItemDelta{Kind: ReasoningDeltaKind, Text: text},
	})
}

func (r *reducer) closeText() []RunEvent {
	if r.text == nil {
		return nil
	}
	event := ItemCompleted{Item: transcript.Item{
		ID: r.text.id, RunID: r.cfg.RunID, Status: transcript.ItemCompleted,
		Kind: transcript.AgentMessage, CreatedAt: r.text.createdAt,
		Content: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: r.text.buf.String()}},
	}}
	r.text = nil
	return []RunEvent{event}
}

func (r *reducer) closeReasoning() []RunEvent {
	if r.reasoning == nil {
		return nil
	}
	event := ItemCompleted{Item: transcript.Item{
		ID: r.reasoning.id, RunID: r.cfg.RunID, Status: transcript.ItemCompleted,
		Kind: transcript.Reasoning, CreatedAt: r.reasoning.createdAt,
		Text: r.reasoning.buf.String(),
	}}
	r.reasoning = nil
	return []RunEvent{event}
}

func (r *reducer) closeStreaming() []RunEvent {
	return append(r.closeReasoning(), r.closeText()...)
}

func (r *reducer) toolStart(e ToolCallStart) ([]RunEvent, error) {
	if strings.TrimSpace(e.CallID) == "" {
		return nil, errors.New("tool call id is required")
	}
	if strings.TrimSpace(e.ToolName) == "" {
		return nil, errors.New("tool name is required")
	}
	if _, duplicate := r.tools[e.CallID]; duplicate {
		return nil, fmt.Errorf("tool call %q started more than once", e.CallID)
	}
	arguments, err := parseToolArguments(e.Arguments)
	if err != nil {
		return nil, fmt.Errorf("tool %q arguments: %w", e.ToolName, err)
	}
	if err := r.resume.rejectCommittedToolStart(e.CallID, e.ToolName, arguments); err != nil {
		return nil, err
	}
	out := r.closeStreaming()
	r.toolOrder++
	// The step number previews the Run's accounting, so it counts the same thing
	// the committed metrics do. Reporting the segment's own count would make a
	// resumed Run appear to start over at step 1.
	step := r.metrics().Steps
	out = append(out, SegmentProgressed{Progress: RunProgress{
		Step: &step, ToolName: e.ToolName, Activity: e.Activity,
	}})
	ref := &openTool{
		callID: e.CallID, sourceCallID: e.SourceCallID, order: r.toolOrder,
		id: r.reuseOrNextItemID(e.CallID, e.ToolName, arguments), createdAt: r.now(),
		name: e.ToolName, arguments: arguments, safetyClass: e.SafetyClass,
	}
	r.tools.add(ref)
	out = append(out, ItemStarted{Item: r.runningToolItem(ref)})
	if e.Arguments != "" {
		out = append(out, ItemChanged{
			ItemID: ref.id,
			Delta:  ItemDelta{Kind: ToolArgumentsDelta, ArgumentsTextDelta: e.Arguments},
		})
	}
	return out, nil
}

func (r *reducer) runningToolItem(ref *openTool) transcript.Item {
	return transcript.Item{
		ID: ref.id, RunID: r.cfg.RunID, Status: transcript.ItemRunning,
		Kind: transcript.ToolCall, CreatedAt: ref.createdAt,
		Tool:        newToolInvocation(ref.name, ref.arguments, nil),
		SafetyClass: ref.safetyClass,
	}
}

func (r *reducer) openToolItemID(callID string) (string, bool) {
	ref, open := r.tools[callID]
	if !open || ref == nil {
		return "", false
	}
	return ref.id, true
}

// spawningItem resolves the executor's immutable parent-call identity to the
// canonical running Item that represents it. Only currently open calls are
// eligible: an AgentTool creates its child before that parent call can finish.
// It returns the complete canonical Item because child admission must persist
// that Item in the same transaction as the child's lineage edge. Ambiguity is
// rejected rather than resolved by ordering.
func (r *reducer) spawningItem(sourceCallID string) (transcript.Item, error) {
	if strings.TrimSpace(sourceCallID) == "" {
		return transcript.Item{}, errors.New("spawning source call id is required")
	}
	var match *openTool
	for _, candidate := range r.tools {
		if candidate.sourceCallID != sourceCallID {
			continue
		}
		if match != nil {
			return transcript.Item{}, fmt.Errorf("source call %q identifies multiple open tool items", sourceCallID)
		}
		match = candidate
	}
	if match == nil {
		return transcript.Item{}, fmt.Errorf("source call %q has no open tool item", sourceCallID)
	}
	return r.runningToolItem(match), nil
}

func (r *reducer) toolEnd(e ToolCallEnd) ([]RunEvent, error) {
	ref, ok := r.tools[e.CallID]
	if !ok {
		if consumed, err := r.resume.consumeCommittedTool(e); consumed {
			return nil, err
		}
		return nil, fmt.Errorf("tool call %q ended without an open start", e.CallID)
	}
	if ref.end != nil {
		return nil, fmt.Errorf("tool call %q ended more than once", e.CallID)
	}
	copy := e
	if e.Offload != nil {
		ref := *e.Offload
		copy.Offload = &ref
	}
	if e.Problem != nil {
		problem := *e.Problem
		copy.Problem = &problem
	}
	copy.MutatedPaths = slices.Clone(e.MutatedPaths)
	ref.end = &copy
	return r.flushEndedTools()
}

// flushEndedTools commits only the longest completed prefix. Tools may finish
// concurrently in any order, but transcript identity, mutation nudges, and
// durable insertion order must follow the model's call order.
func (r *reducer) flushEndedTools() ([]RunEvent, error) {
	ordered := r.tools.ordered()
	var out []RunEvent
	for _, ref := range ordered {
		if ref.end == nil {
			break
		}
		delete(r.tools, ref.callID)
		completed, err := r.completeTool(ref, *ref.end)
		if err != nil {
			return nil, err
		}
		out = append(out, completed...)
	}
	return out, nil
}

func (r *reducer) completeTool(ref *openTool, e ToolCallEnd) ([]RunEvent, error) {
	var out []RunEvent
	if e.OutputText != "" {
		out = append(out, ItemChanged{
			ItemID: ref.id,
			Delta:  ItemDelta{Kind: ToolOutputDelta, Text: e.OutputText},
		})
	}
	arguments := ref.arguments
	if e.Arguments != "" {
		parsed, err := parseToolArguments(e.Arguments)
		if err != nil {
			return nil, fmt.Errorf("tool %q effective arguments: %w", ref.name, err)
		}
		arguments = parsed
	}
	invocation := newToolInvocation(ref.name, arguments, e.Result)
	invocation.Offload = e.Offload
	item := transcript.Item{
		ID: ref.id, RunID: r.cfg.RunID, Status: transcript.ItemCompleted,
		Kind: transcript.ToolCall, CreatedAt: ref.createdAt,
		Tool:        invocation,
		SafetyClass: ref.safetyClass,
	}
	if e.Problem != nil {
		if err := e.Problem.ValidateFor(transcript.ToolProblem); err != nil {
			return nil, fmt.Errorf("tool %q problem: %w", ref.name, err)
		}
		item.Status = transcript.ItemIncomplete
		problem := *e.Problem
		item.Error = &problem
	}
	return append(out, ItemCompleted{Item: item, mutatedPaths: e.MutatedPaths}), nil
}

// usageProgress records the executor's latest accounting report and previews the
// Run's resulting total. The report is remembered rather than only forwarded:
// it is what the Run commits if the segment ends without a fresh one.
func (r *reducer) usageProgress(e UsageReported) ([]RunEvent, error) {
	if err := r.applyUsage(TurnUsage{
		Tokens:  e.TokenUsage,
		ByModel: e.ByModel,
		CostUSD: e.CostUSD,
		Steps:   e.Steps,
	}); err != nil {
		return nil, err
	}
	progress := RunProgress{Usage: r.metrics().Usage}
	if e.ContextTokens > 0 {
		contextTokens := e.ContextTokens
		progress.ContextTokens = &contextTokens
	}
	step := r.step
	progress.Step = &step
	return []RunEvent{SegmentProgressed{Progress: progress}}, nil
}

func (r *reducer) compaction(e CompactBoundary) []RunEvent {
	dropped := max(e.MessagesBefore-e.MessagesAfter, 0)
	id, now := r.nextItemID(), r.now()
	return itemPair(func(status transcript.ItemStatus) transcript.Item {
		return transcript.Item{
			ID: id, RunID: r.cfg.RunID, Status: status,
			Kind: transcript.Compaction, CreatedAt: now, DroppedMessages: dropped,
		}
	})
}

func (r *reducer) openUserMessage() []RunEvent {
	if len(r.userInput) == 0 {
		return nil
	}
	input := r.userInput
	r.userInput = nil
	id, now := userMessageItemID(r.cfg.SegmentID), r.now()
	return itemPair(func(status transcript.ItemStatus) transcript.Item {
		return transcript.Item{
			ID: id, RunID: r.cfg.RunID, Status: status,
			Kind: transcript.UserMessage, CreatedAt: now, Content: input,
		}
	})
}

func (r *reducer) steerMessage(e SteerMessage) []RunEvent {
	out := r.closeStreaming()
	id, now := r.nextItemID(), r.now()
	content := append([]transcript.ContentBlock(nil), e.Content...)
	events := itemPair(func(status transcript.ItemStatus) transcript.Item {
		return transcript.Item{
			ID: id, RunID: r.cfg.RunID, Status: status,
			Kind: transcript.UserMessage, CreatedAt: now,
			Content: content,
		}
	})
	return append(out, events...)
}

func (r *reducer) todosSnapshot(e TodosUpdated) []RunEvent {
	snapshot := r.todoState(e.State)
	// Remembered so the segment can fence its final value: a client folding this
	// stream must reach segment.finished holding the state the segment ended with,
	// not the state as of whichever change happened to be published last.
	r.todos = &snapshot
	return []RunEvent{snapshot}
}

func (r *reducer) todoState(state todo.State) StateSnapshot {
	todos := make([]TodoSnapshot, len(state.Items))
	for i, item := range state.Items {
		todos[i] = TodoSnapshot{
			ID: strconv.Itoa(i), Text: item.Content, Status: item.Status,
			BlockedReason: item.BlockedReason, NextAction: item.NextAction,
		}
	}
	return StateSnapshot{
		SessionID: r.cfg.SessionID, Todos: todos,
		Revision: state.Revision, UpdatedAt: state.UpdatedAt,
	}
}
