package runs

import (
	"strconv"
)

func itemPair(build func(ItemStatus) Item) []RunEvent {
	return []RunEvent{
		ItemStarted{Item: build(ItemRunning)},
		ItemCompleted{Item: build(ItemSucceeded)},
	}
}

func (r *reducer) appendText(text string) []RunEvent {
	var out []RunEvent
	if r.text == nil {
		r.text = &openText{id: r.nextItemID(), createdAt: r.now()}
		out = append(out, ItemStarted{Item: Item{
			ID: r.text.id, RunID: r.cfg.RunID, Status: ItemRunning,
			Kind: AgentMessage, CreatedAt: r.text.createdAt,
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
		out = append(out, ItemStarted{Item: Item{
			ID: r.reasoning.id, RunID: r.cfg.RunID, Status: ItemRunning,
			Kind: Reasoning, CreatedAt: r.reasoning.createdAt,
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
	event := ItemCompleted{Item: Item{
		ID: r.text.id, RunID: r.cfg.RunID, Status: ItemSucceeded,
		Kind: AgentMessage, CreatedAt: r.text.createdAt,
		Content: []ContentBlock{{Kind: TextContent, Text: r.text.buf.String()}},
	}}
	r.text = nil
	return []RunEvent{event}
}

func (r *reducer) closeReasoning() []RunEvent {
	if r.reasoning == nil {
		return nil
	}
	event := ItemCompleted{Item: Item{
		ID: r.reasoning.id, RunID: r.cfg.RunID, Status: ItemSucceeded,
		Kind: Reasoning, CreatedAt: r.reasoning.createdAt,
		Text: r.reasoning.buf.String(),
	}}
	r.reasoning = nil
	return []RunEvent{event}
}

func (r *reducer) closeStreaming() []RunEvent {
	return append(r.closeReasoning(), r.closeText()...)
}

func (r *reducer) toolStart(e ToolCallStart) []RunEvent {
	out := r.closeStreaming()
	r.step++
	step := r.step
	r.toolOrder++
	out = append(out, SegmentProgressed{Progress: RunProgress{
		Step: &step, Activity: activityVerb(e.ToolName),
	}})
	ref := &openTool{
		callID: e.CallID, order: r.toolOrder,
		id: r.reuseOrNextItemID(e.ToolName, e.Arguments), createdAt: r.now(),
		name: e.ToolName, args: e.Arguments, safetyClass: e.SafetyClass,
	}
	r.tools[e.CallID] = ref
	out = append(out, ItemStarted{Item: Item{
		ID: ref.id, RunID: r.cfg.RunID, Status: ItemRunning,
		Kind: ToolCall, CreatedAt: ref.createdAt,
		Tool:        r.newToolInvocation(e.ToolName, e.Arguments, ""),
		SafetyClass: e.SafetyClass,
	}})
	if e.Arguments != "" {
		out = append(out, ItemChanged{
			ItemID: ref.id,
			Delta:  ItemDelta{Kind: ToolArgumentsDelta, ArgumentsTextDelta: e.Arguments},
		})
	}
	return out
}

func (r *reducer) toolEnd(e ToolCallEnd) []RunEvent {
	ref, ok := r.tools[e.CallID]
	if !ok {
		return nil
	}
	delete(r.tools, e.CallID)
	var out []RunEvent
	if isCommandTool(ref.name) && e.Output != "" {
		if merged := commandOutput(e.Output); merged != "" {
			out = append(out, ItemChanged{
				ItemID: ref.id,
				Delta:  ItemDelta{Kind: ToolOutputDelta, Text: merged},
			})
		}
	}
	item := Item{
		ID: ref.id, RunID: r.cfg.RunID, Status: ItemSucceeded,
		Kind: ToolCall, CreatedAt: ref.createdAt,
		Tool:        r.newToolInvocation(ref.name, ref.args, e.Output),
		SafetyClass: ref.safetyClass,
	}
	switch {
	case e.Denied:
		item.Status = ItemIncomplete
		item.Error = &Problem{Kind: DeniedByUserProblem, Scope: ToolProblem, Detail: "tool call denied by user"}
	case e.Err != "":
		item.Status = ItemIncomplete
		item.Error = &Problem{Kind: ToolFailedProblem, Scope: ToolProblem, Detail: e.Err}
	}
	return append(out, ItemCompleted{Item: item})
}

func (r *reducer) usageProgress(e UsageReported) []RunEvent {
	progress := RunProgress{Usage: &Usage{ModelUsage: modelUsageFrom(
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
	return itemPair(func(status ItemStatus) Item {
		return Item{
			ID: id, RunID: r.cfg.RunID, Status: status,
			Kind: Compaction, CreatedAt: now, DroppedMessages: dropped,
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
	return itemPair(func(status ItemStatus) Item {
		return Item{
			ID: id, RunID: r.cfg.RunID, Status: status,
			Kind: UserMessage, CreatedAt: now, Content: input,
		}
	})
}

func (r *reducer) steerMessage(e SteerMessage) []RunEvent {
	out := r.closeStreaming()
	id, now := r.nextItemID(), r.now()
	events := itemPair(func(status ItemStatus) Item {
		return Item{
			ID: id, RunID: r.cfg.RunID, Status: status,
			Kind: UserMessage, CreatedAt: now,
			Content: []ContentBlock{{Kind: TextContent, Text: e.Text}},
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
	default:
		return "Calling " + name
	}
}
