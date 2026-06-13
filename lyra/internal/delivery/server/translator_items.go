package server

import (
	"time"

	"github.com/Tangerg/lynx/lyra/internal/delivery/protocol"
	"github.com/Tangerg/lynx/lyra/internal/kernel/chat"
)

func (t *translator) appendText(text string) []protocol.StreamEvent {
	var out []protocol.StreamEvent
	if t.text == nil {
		t.text = &openText{id: t.nextItemID(), createdAt: time.Now().UTC()}
		out = append(out, protocol.StreamEvent{
			Type: protocol.StreamItemStarted,
			Item: &protocol.Item{
				ID:        t.text.id,
				RunID:     t.runID,
				Status:    protocol.ItemStatusRunning,
				Type:      protocol.ItemTypeAgentMessage,
				CreatedAt: t.text.createdAt,
			},
		})
	}
	t.text.buf.WriteString(text)
	idx := 0
	return append(out, protocol.StreamEvent{
		Type:   protocol.StreamItemDelta,
		ItemID: t.text.id,
		Delta:  &protocol.ItemDelta{Type: protocol.DeltaContent, Index: &idx, Text: text},
	})
}

func (t *translator) appendReasoning(text string) []protocol.StreamEvent {
	var out []protocol.StreamEvent
	if t.reasoning == nil {
		t.reasoning = &openText{id: t.nextItemID(), createdAt: time.Now().UTC()}
		out = append(out, protocol.StreamEvent{
			Type: protocol.StreamItemStarted,
			Item: &protocol.Item{
				ID:        t.reasoning.id,
				RunID:     t.runID,
				Status:    protocol.ItemStatusRunning,
				Type:      protocol.ItemTypeReasoning,
				CreatedAt: t.reasoning.createdAt,
			},
		})
	}
	t.reasoning.buf.WriteString(text)
	return append(out, protocol.StreamEvent{
		Type:   protocol.StreamItemDelta,
		ItemID: t.reasoning.id,
		Delta:  &protocol.ItemDelta{Type: protocol.DeltaReasoning, Text: text},
	})
}

func (t *translator) closeText() []protocol.StreamEvent {
	if t.text == nil {
		return nil
	}
	item := &protocol.Item{
		ID:        t.text.id,
		RunID:     t.runID,
		Status:    protocol.ItemStatusCompleted,
		Type:      protocol.ItemTypeAgentMessage,
		CreatedAt: t.text.createdAt,
		Content:   []protocol.ContentBlock{{Type: "text", Text: t.text.buf.String()}},
	}
	t.text = nil
	return []protocol.StreamEvent{{Type: protocol.StreamItemCompleted, Item: item}}
}

func (t *translator) closeReasoning() []protocol.StreamEvent {
	if t.reasoning == nil {
		return nil
	}
	item := &protocol.Item{
		ID:        t.reasoning.id,
		RunID:     t.runID,
		Status:    protocol.ItemStatusCompleted,
		Type:      protocol.ItemTypeReasoning,
		CreatedAt: t.reasoning.createdAt,
		Text:      t.reasoning.buf.String(),
	}
	t.reasoning = nil
	return []protocol.StreamEvent{{Type: protocol.StreamItemCompleted, Item: item}}
}

func (t *translator) toolStart(e chat.ToolCallStart) []protocol.StreamEvent {
	out := t.closeReasoning()
	out = append(out, t.closeText()...)

	id, runID := t.reuseOrNextItemID(e.ToolName, e.Arguments)
	ref := &openTool{id: id, runID: runID, createdAt: time.Now().UTC(), name: e.ToolName, args: e.Arguments}
	t.tools[e.CallID] = ref
	out = append(out, protocol.StreamEvent{
		Type: protocol.StreamItemStarted,
		Item: &protocol.Item{
			ID:        ref.id,
			RunID:     ref.runID,
			Status:    protocol.ItemStatusRunning,
			Type:      protocol.ItemTypeToolCall,
			CreatedAt: ref.createdAt,
			Tool:      t.newToolInvocation(e.ToolName, e.Arguments, ""),
		},
	})
	if e.Arguments != "" {
		out = append(out, protocol.StreamEvent{
			Type:   protocol.StreamItemDelta,
			ItemID: ref.id,
			Delta:  &protocol.ItemDelta{Type: protocol.DeltaToolArguments, ArgumentsTextDelta: e.Arguments},
		})
	}
	return out
}

func (t *translator) toolEnd(e chat.ToolCallEnd) []protocol.StreamEvent {
	ref, ok := t.tools[e.CallID]
	if !ok {
		return nil
	}
	delete(t.tools, e.CallID)

	var out []protocol.StreamEvent
	// The authoritative command output lands on the completed item's
	// tool.result.output (durable, below); this toolOutput delta is only its
	// streaming preview (API.md §4.4 / §5.2). Same merged stdout+stderr text
	// so preview and terminal agree.
	if isCommandTool(ref.name) && e.Output != "" {
		if merged := commandOutput(e.Output); merged != "" {
			out = append(out, protocol.StreamEvent{
				Type:   protocol.StreamItemDelta,
				ItemID: ref.id,
				Delta:  &protocol.ItemDelta{Type: protocol.DeltaToolOutput, Text: merged},
			})
		}
	}

	item := &protocol.Item{
		ID:        ref.id,
		RunID:     ref.runID,
		Status:    protocol.ItemStatusCompleted,
		Type:      protocol.ItemTypeToolCall,
		CreatedAt: ref.createdAt,
		Tool:      t.newToolInvocation(ref.name, ref.args, e.Output),
	}
	switch {
	case e.Denied:
		// Denied by the approval verdict — a distinct terminal from a green
		// success or a generic failure, so the UI can render "denied".
		item.Status = protocol.ItemStatusIncomplete
		item.Error = &protocol.ProblemData{Type: "denied_by_user", Channel: "tool", Detail: "tool call denied by user"}
	case e.Err != "":
		item.Status = protocol.ItemStatusIncomplete
		item.Error = &protocol.ProblemData{Type: "tool_failed", Channel: "tool", Detail: e.Err}
	}
	return append(out, protocol.StreamEvent{Type: protocol.StreamItemCompleted, Item: item})
}

// turnEnd closes any open items (so the wire ends balanced) then emits
// the terminal run.finished with its discriminated outcome.
func (t *translator) turnEnd(e chat.TurnEnd) []protocol.StreamEvent {
	out := t.closeReasoning()
	out = append(out, t.closeText()...)
	out = append(out, t.drainTools()...)
	return append(out, protocol.StreamEvent{
		Type:    protocol.StreamRunFinished,
		Outcome: t.outcome(e),
	})
}

// finish builds a terminal run.finished for paths that never observe a
// chat.TurnEnd (e.g. run cancellation drained the iterator). Closes
// open items, then emits run.finished with the given outcome type.
func (t *translator) finish(outcomeType protocol.RunOutcomeType) []protocol.StreamEvent {
	out := t.closeReasoning()
	out = append(out, t.closeText()...)
	out = append(out, t.drainTools()...)
	res := &protocol.RunResult{}
	if outcomeType == protocol.OutcomeError && t.errMsg != "" {
		res.Error = t.classifyRunError(t.errMsg)
	}
	return append(out, protocol.StreamEvent{
		Type:    protocol.StreamRunFinished,
		Outcome: &protocol.RunOutcome{Type: outcomeType, Result: res},
	})
}
