package runs

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
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
	out := r.closeStreaming()
	r.step++
	step := r.step
	r.toolOrder++
	out = append(out, SegmentProgressed{Progress: RunProgress{
		Step: &step, ToolName: e.ToolName, Activity: e.Activity,
	}})
	ref := &openTool{
		callID: e.CallID, order: r.toolOrder,
		id: r.reuseOrNextItemID(e.CallID, e.ToolName, arguments), createdAt: r.now(),
		name: e.ToolName, arguments: arguments, safetyClass: e.SafetyClass,
	}
	r.tools.add(ref)
	out = append(out, ItemStarted{Item: transcript.Item{
		ID: ref.id, RunID: r.cfg.RunID, Status: transcript.ItemRunning,
		Kind: transcript.ToolCall, CreatedAt: ref.createdAt,
		Tool:        newToolInvocation(e.ToolName, arguments, nil),
		SafetyClass: e.SafetyClass,
	}})
	if e.Arguments != "" {
		out = append(out, ItemChanged{
			ItemID: ref.id,
			Delta:  ItemDelta{Kind: ToolArgumentsDelta, ArgumentsTextDelta: e.Arguments},
		})
	}
	return out, nil
}

func (r *reducer) toolEnd(e ToolCallEnd) ([]RunEvent, error) {
	ref, ok := r.tools[e.CallID]
	if !ok {
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
	switch {
	case e.Denied:
		item.Status = transcript.ItemIncomplete
		item.Error = &transcript.Problem{Kind: transcript.DeniedByUserProblem, Scope: transcript.ToolProblem, Detail: "tool call denied by user"}
	case e.Err != "":
		item.Status = transcript.ItemIncomplete
		item.Error = &transcript.Problem{Kind: transcript.ToolFailedProblem, Scope: transcript.ToolProblem, Detail: e.Err}
	}
	return append(out, ItemCompleted{Item: item, mutatedPaths: e.MutatedPaths}), nil
}

func (r *reducer) usageProgress(e UsageReported) []RunEvent {
	progress := RunProgress{Usage: &transcript.Usage{ModelUsage: modelUsageFrom(
		e.TokenUsage.PromptTokens,
		e.TokenUsage.CompletionTokens,
		e.TokenUsage.ReasoningTokens,
		e.TokenUsage.CacheReadTokens,
		e.TokenUsage.CacheWriteTokens,
		e.CostUSD,
	)}}
	if e.ContextTokens > 0 {
		contextTokens := e.ContextTokens
		progress.ContextTokens = &contextTokens
	}
	return []RunEvent{SegmentProgressed{Progress: progress}}
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
	events := itemPair(func(status transcript.ItemStatus) transcript.Item {
		return transcript.Item{
			ID: id, RunID: r.cfg.RunID, Status: status,
			Kind: transcript.UserMessage, CreatedAt: now,
			Content: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: e.Text}},
		}
	})
	return append(out, events...)
}

func (r *reducer) todosSnapshot(e TodosUpdated) []RunEvent {
	todos := make([]TodoSnapshot, len(e.Todos))
	for i, item := range e.Todos {
		todos[i] = TodoSnapshot{
			ID: strconv.Itoa(i), Text: item.Content, Status: string(item.Status),
			BlockedReason: item.BlockedReason, NextAction: item.NextAction,
		}
	}
	return []RunEvent{StateSnapshot{Todos: todos}}
}
