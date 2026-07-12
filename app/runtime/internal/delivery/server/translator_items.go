package server

import (
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
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
		Content:   []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: t.text.buf.String()}},
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

// closeStreaming flushes any open reasoning then text stream as completed items
// — the ordered pair emitted at every boundary (tool start, turn end, interrupt,
// steer) before the next event.
func (t *translator) closeStreaming() []protocol.StreamEvent {
	return append(t.closeReasoning(), t.closeText()...)
}

func (t *translator) toolStart(e turn.ToolCallStart) []protocol.StreamEvent {
	out := t.closeStreaming()

	// Mid-run progress (API.md §5, ephemeral): a tool call is a meaningful
	// activity boundary, so surface "what's happening now" + the running tool
	// ordinal. Text/reasoning deltas are their OWN activity signal, so
	// run.progress fires only here — not per high-frequency delta.
	t.step++
	step := t.step
	out = append(out, protocol.StreamEvent{
		Type:     protocol.StreamRunProgress,
		Progress: &protocol.RunProgress{Step: &step, Activity: activityVerb(e.ToolName)},
	})

	ref := &openTool{id: t.reuseOrNextItemID(e.ToolName, e.Arguments), createdAt: time.Now().UTC(), name: e.ToolName, args: e.Arguments, safetyClass: e.SafetyClass}
	t.tools[e.CallID] = ref
	out = append(out, protocol.StreamEvent{
		Type: protocol.StreamItemStarted,
		Item: &protocol.Item{
			ID:          ref.id,
			RunID:       t.runID,
			Status:      protocol.ItemStatusRunning,
			Type:        protocol.ItemTypeToolCall,
			CreatedAt:   ref.createdAt,
			Tool:        t.newToolInvocation(e.ToolName, e.Arguments, ""),
			SafetyClass: protocol.SafetyClass(e.SafetyClass),
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

func (t *translator) toolEnd(e turn.ToolCallEnd) []protocol.StreamEvent {
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
		ID:          ref.id,
		RunID:       t.runID,
		Status:      protocol.ItemStatusCompleted,
		Type:        protocol.ItemTypeToolCall,
		CreatedAt:   ref.createdAt,
		Tool:        t.newToolInvocation(ref.name, ref.args, e.Output),
		SafetyClass: protocol.SafetyClass(ref.safetyClass),
	}
	switch {
	case e.Denied:
		// Denied by the approval verdict — a distinct terminal from a green
		// success or a generic failure, so the UI can render "denied".
		item.Status = protocol.ItemStatusIncomplete
		item.Error = &protocol.ProblemData{Type: protocol.ProblemDeniedByUser, Channel: protocol.ErrorChannelTool, Detail: "tool call denied by user"}
	case e.Err != "":
		item.Status = protocol.ItemStatusIncomplete
		item.Error = &protocol.ProblemData{Type: protocol.ProblemToolFailed, Channel: protocol.ErrorChannelTool, Detail: e.Err}
	}
	return append(out, protocol.StreamEvent{Type: protocol.StreamItemCompleted, Item: item})
}

// usageProgress surfaces a per-round cumulative usage report as a run.progress
// preview (API.md §5, ephemeral) — the live "tokens / cost spent" readout. Only
// the usage field is carried; step/activity ride the tool-call boundary above.
// The authoritative final total still lands on run.finished.result (§5.2).
func (t *translator) usageProgress(e turn.UsageReported) []protocol.StreamEvent {
	progress := &protocol.RunProgress{
		Usage: &protocol.Usage{
			ModelUsage: modelUsageFrom(e.TokenUsage.PromptTokens, e.TokenUsage.CompletionTokens, e.TokenUsage.ReasoningTokens, e.TokenUsage.CacheReadTokens, e.TokenUsage.CacheWriteTokens, e.CostUSD),
		},
	}
	// Live context occupancy = this round's prompt size. Omit a zero (a provider
	// that didn't report prompt tokens) rather than show an empty gauge.
	if e.ContextTokens > 0 {
		ct := e.ContextTokens
		progress.ContextTokens = &ct
	}
	return []protocol.StreamEvent{{Type: protocol.StreamRunProgress, Progress: progress}}
}

// activityVerb maps a tool name to a human-readable mid-run activity line for
// run.progress (API.md §5) — the "what's happening now" a client shows while
// the tool runs. A small first-party verb map with a generic "Calling <name>"
// fallback (covers MCP "<server>.<tool>" and any dynamic / lsp_* tool).
func activityVerb(name string) string {
	switch name {
	case "shell", "run_in_background":
		return "Running command"
	case "shell_output":
		return "Reading command output"
	case "shell_kill":
		return "Stopping command"
	case "read":
		return "Reading file"
	case "write":
		return "Writing file"
	case "edit":
		return "Editing file"
	case "grep":
		return "Searching"
	case "glob":
		return "Finding files"
	case "web_search":
		return "Searching the web"
	case "web_fetch":
		return "Fetching a page"
	case "task", "subagent":
		return "Delegating to a sub-agent"
	case "ask_user":
		return "Waiting for your answer"
	case "todo_write":
		return "Updating the plan"
	}
	return "Calling " + name
}

// turnEnd closes any open items (so the wire ends balanced) then emits
// the terminal run.finished with its discriminated outcome.
func (t *translator) turnEnd(e turn.TurnEnd) []protocol.StreamEvent {
	out := t.closeStreaming()
	out = append(out, t.drainTools()...)
	return append(out, protocol.StreamEvent{
		Type:    protocol.StreamRunFinished,
		Outcome: t.outcome(e),
	})
}

// finish builds a terminal run.finished for paths that never observe a
// turn.TurnEnd (e.g. run cancellation drained the iterator). Closes
// open items, then emits run.finished with the given outcome type.
func (t *translator) finish(outcomeType protocol.RunOutcomeType) []protocol.StreamEvent {
	out := t.closeStreaming()
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
