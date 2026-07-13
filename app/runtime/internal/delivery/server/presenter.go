package server

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

func presentRunEvent(event runs.RunEvent) protocol.StreamEvent {
	switch event := event.(type) {
	case runs.SegmentStarted:
		run := presentRun(event.Run)
		return protocol.StreamEvent{Type: protocol.StreamSegmentStarted, Run: &run}
	case runs.SegmentProgressed:
		progress := presentProgress(event.Progress)
		return protocol.StreamEvent{Type: protocol.StreamSegmentProgress, Progress: &progress}
	case runs.SegmentFinished:
		outcome := presentOutcome(event.Run)
		return protocol.StreamEvent{Type: protocol.StreamSegmentFinished, Outcome: &outcome}
	case runs.ItemStarted:
		item := presentItem(event.Item)
		return protocol.StreamEvent{Type: protocol.StreamItemStarted, Item: &item}
	case runs.ItemChanged:
		delta := presentDelta(event.Delta)
		return protocol.StreamEvent{Type: protocol.StreamItemDelta, ItemID: event.ItemID, Delta: &delta}
	case runs.ItemCompleted:
		item := presentItem(event.Item)
		return protocol.StreamEvent{Type: protocol.StreamItemCompleted, Item: &item}
	case runs.StateSnapshot:
		todos := make([]protocol.TodoSnapshot, len(event.Todos))
		for i, todo := range event.Todos {
			todos[i] = protocol.TodoSnapshot{
				ID: todo.ID, Text: todo.Text, Status: todo.Status,
				BlockedReason: todo.BlockedReason, NextAction: todo.NextAction,
			}
		}
		return protocol.StreamEvent{Type: protocol.StreamStateSnapshot, State: map[string]any{"todos": todos}}
	default:
		panic("server: unknown canonical run event")
	}
}

func presentRun(run transcript.Run) protocol.RunRef {
	status := protocol.RunStatusFinished
	if run.State == execution.Running {
		status = protocol.RunStatusRunning
	}
	ref := protocol.RunRef{
		ID: run.ID, SessionID: run.SessionID, SpawnedByItemID: run.SpawnedByItemID,
		Provider: run.Provider, Model: run.Model, Status: status,
		CreatedAt: run.CreatedAt, FinishedAt: run.FinishedAt,
	}
	if run.State != execution.Running {
		outcome := presentOutcome(run)
		ref.Outcome = &outcome
	}
	return ref
}

func presentOutcome(run transcript.Run) protocol.RunOutcome {
	if run.State == execution.Interrupted {
		return protocol.RunOutcome{Type: protocol.OutcomeInterrupt, Interrupts: presentInterrupts(run.Interrupts)}
	}
	kind := protocol.OutcomeCompleted
	if run.Outcome != nil {
		switch *run.Outcome {
		case execution.OutcomeCanceled:
			kind = protocol.OutcomeCanceled
		case execution.OutcomeError:
			kind = protocol.OutcomeError
		case execution.OutcomeMaxBudget:
			kind = protocol.OutcomeMaxBudget
		case execution.OutcomeMaxSteps:
			kind = protocol.OutcomeMaxSteps
		}
	}
	return protocol.RunOutcome{Type: kind, Result: presentRunResult(run.Result), Detail: run.Detail}
}

func presentRunResult(result *transcript.RunResult) *protocol.RunResult {
	if result == nil {
		return nil
	}
	steps := result.Steps
	return &protocol.RunResult{
		Usage: presentUsage(result.Usage), Steps: &steps,
		Error: presentProblem(result.Error), DurationMs: int(result.Duration.Milliseconds()),
	}
}

func presentProgress(progress runs.RunProgress) protocol.RunProgress {
	return protocol.RunProgress{
		Step: progress.Step, MaxSteps: progress.MaxSteps,
		Usage: presentUsage(progress.Usage), ContextTokens: progress.ContextTokens,
		Activity: progress.Activity,
	}
}

func presentUsage(usage *transcript.Usage) *protocol.Usage {
	if usage == nil {
		return nil
	}
	out := &protocol.Usage{ModelUsage: presentModelUsage(usage.ModelUsage)}
	if len(usage.ByModel) > 0 {
		out.ByModel = make(map[string]protocol.ModelUsage, len(usage.ByModel))
		for model, modelUsage := range usage.ByModel {
			out.ByModel[model] = presentModelUsage(modelUsage)
		}
	}
	return out
}

func presentModelUsage(usage transcript.ModelUsage) protocol.ModelUsage {
	return protocol.ModelUsage{
		InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens,
		CacheReadTokens: usage.CacheReadTokens, CacheWriteTokens: usage.CacheWriteTokens,
		ReasoningTokens: usage.ReasoningTokens, CostUSD: usage.CostUSD,
	}
}

func presentItem(item transcript.Item) protocol.Item {
	out := protocol.Item{
		ID: item.ID, RunID: item.RunID, Status: presentItemStatus(item.Status),
		CreatedAt: item.CreatedAt, Type: presentItemKind(item.Kind),
		Text: item.Text, Redacted: item.Redacted,
		SafetyClass: protocol.SafetyClass(item.SafetyClass), Error: presentProblem(item.Error),
		Summary: item.Summary, DroppedMessages: item.DroppedMessages,
	}
	if len(item.Content) > 0 {
		out.Content = make([]protocol.ContentBlock, len(item.Content))
		for i, block := range item.Content {
			out.Content[i] = presentContent(block)
		}
	}
	if len(item.Steps) > 0 {
		out.Steps = presentPlanSteps(item.Steps)
	}
	if item.Question != nil {
		question := presentQuestion(*item.Question)
		out.Question = &question
	}
	if item.Tool != nil {
		tool := presentTool(*item.Tool)
		out.Tool = &tool
	}
	return out
}

func presentItemStatus(status transcript.ItemStatus) protocol.ItemStatus {
	switch status {
	case transcript.ItemCompleted:
		return protocol.ItemStatusCompleted
	case transcript.ItemIncomplete:
		return protocol.ItemStatusIncomplete
	default:
		return protocol.ItemStatusRunning
	}
}

func presentItemKind(kind transcript.ItemKind) protocol.ItemType {
	switch kind {
	case transcript.AgentMessage:
		return protocol.ItemTypeAgentMessage
	case transcript.Reasoning:
		return protocol.ItemTypeReasoning
	case transcript.Plan:
		return protocol.ItemTypePlan
	case transcript.QuestionItem:
		return protocol.ItemTypeQuestion
	case transcript.ToolCall:
		return protocol.ItemTypeToolCall
	case transcript.Compaction:
		return protocol.ItemTypeCompaction
	default:
		return protocol.ItemTypeUserMessage
	}
}

func presentContent(block transcript.ContentBlock) protocol.ContentBlock {
	kind := protocol.ContentBlockText
	if block.Kind == transcript.ImageContent {
		kind = protocol.ContentBlockImage
	}
	return protocol.ContentBlock{Type: kind, Text: block.Text, Mime: block.Mime, Data: block.Data}
}

func presentPlanSteps(steps []transcript.PlanStep) []protocol.PlanStep {
	out := make([]protocol.PlanStep, len(steps))
	for i, step := range steps {
		out[i] = protocol.PlanStep{ID: step.ID, Title: step.Title, Status: protocol.PlanStepStatus(step.Status)}
	}
	return out
}

func presentQuestion(question transcript.Question) protocol.Question {
	fields := make([]protocol.QuestionField, len(question.Fields))
	for i, field := range question.Fields {
		kind := protocol.QuestionFieldText
		if field.Kind == transcript.QuestionChoice {
			kind = protocol.QuestionFieldChoice
		}
		options := make([]protocol.QuestionOption, len(field.Options))
		for j, option := range field.Options {
			options[j] = protocol.QuestionOption{
				Label: option.Label, Description: option.Description, Preview: option.Preview,
			}
		}
		fields[i] = protocol.QuestionField{
			Name: field.Name, Label: field.Label, Header: field.Header,
			Required: field.Required, Type: kind, Options: options, Multiple: field.Multiple,
		}
	}
	return protocol.Question{Prompt: question.Prompt, Fields: fields}
}

func presentTool(tool transcript.ToolInvocation) protocol.ToolInvocation {
	return protocol.ToolInvocation{
		Name: tool.Name, Arguments: tool.Arguments, Result: presentToolResult(tool.Result),
	}
}

func presentToolResult(result *transcript.ToolResult) any {
	if result == nil {
		return nil
	}
	switch result.Kind {
	case transcript.CommandToolResult:
		if result.Command == nil {
			return nil
		}
		return struct {
			ExitCode        *int   `json:"exitCode,omitempty"`
			Output          string `json:"output"`
			OutputTruncated bool   `json:"outputTruncated,omitempty"`
		}{result.Command.ExitCode, result.Command.Output, result.Command.OutputTruncated}
	case transcript.SearchToolResult:
		if result.Search == nil {
			return nil
		}
		hits := make([]protocol.SearchHit, len(result.Search.Hits))
		for i, hit := range result.Search.Hits {
			hits[i] = protocol.SearchHit{Path: hit.Path, LineNumber: hit.LineNumber, Snippet: hit.Snippet}
		}
		return struct {
			Hits []protocol.SearchHit `json:"hits"`
		}{hits}
	case transcript.WebSearchToolResult:
		if result.WebSearch == nil {
			return nil
		}
		results := make([]protocol.WebSearchResult, len(result.WebSearch.Results))
		for i, item := range result.WebSearch.Results {
			results[i] = protocol.WebSearchResult{
				Title: item.Title, URL: item.URL, Snippet: item.Snippet, FaviconURL: item.FaviconURL,
			}
		}
		return struct {
			Results []protocol.WebSearchResult `json:"results"`
		}{results}
	case transcript.FileChangeToolResult:
		if result.FileChange == nil {
			return nil
		}
		changes := make([]protocol.FileEdit, len(result.FileChange.Changes))
		for i, change := range result.FileChange.Changes {
			changes[i] = protocol.FileEdit{Path: change.Path, Status: protocol.FileStatus(change.Status), Diff: presentDiff(change.Diff)}
		}
		return struct {
			Changes []protocol.FileEdit `json:"changes"`
		}{changes}
	default:
		return result.Raw
	}
}

func presentDiff(rows []transcript.DiffRow) []protocol.DiffRow {
	out := make([]protocol.DiffRow, len(rows))
	for i, row := range rows {
		kind := protocol.DiffRowContext
		switch row.Kind {
		case transcript.DiffHunk:
			kind = protocol.DiffRowHunk
		case transcript.DiffAdded:
			kind = protocol.DiffRowAdded
		case transcript.DiffDeleted:
			kind = protocol.DiffRowDeleted
		}
		out[i] = protocol.DiffRow{
			Type: kind, Text: row.Text, LeftLine: row.LeftLine,
			RightLine: row.RightLine, Code: row.Code,
		}
	}
	return out
}

func presentProblem(problem *transcript.Problem) *protocol.ProblemData {
	if problem == nil {
		return nil
	}
	kind := protocol.ProblemInternalError
	switch problem.Kind {
	case transcript.RunLostProblem:
		kind = protocol.ProblemRunLost
	case transcript.AgentStuckProblem:
		kind = protocol.ProblemAgentStuck
	case transcript.RateLimitedProblem:
		kind = protocol.ProblemRateLimited
	case transcript.InvalidAPIKeyProblem:
		kind = protocol.ProblemInvalidAPIKey
	case transcript.TimeoutProblem:
		kind = protocol.ProblemTimeout
	case transcript.ProviderUnavailableProblem:
		kind = protocol.ProblemProviderUnavailable
	case transcript.ProviderRejectedProblem:
		kind = protocol.ProblemProviderRejected
	case transcript.DeniedByUserProblem:
		kind = protocol.ProblemDeniedByUser
	case transcript.ToolFailedProblem:
		kind = protocol.ProblemToolFailed
	}
	scope := protocol.ErrorChannelRun
	if problem.Scope == transcript.ToolProblem {
		scope = protocol.ErrorChannelTool
	}
	return &protocol.ProblemData{
		Type: kind, Channel: scope, Detail: problem.Detail, DocURL: problem.DocURL,
		Retryable: problem.Retryable, RetryAfterSeconds: problem.RetryAfterSeconds,
	}
}

func presentInterrupts(interrupts []transcript.Interrupt) []protocol.Interrupt {
	out := make([]protocol.Interrupt, 0, len(interrupts))
	for _, interrupt := range interrupts {
		entry := protocol.Interrupt{ItemID: interrupt.ItemID}
		switch interrupt.Kind {
		case transcript.ApprovalInterrupt:
			if interrupt.Approval == nil {
				continue
			}
			entry.Type = protocol.InterruptApproval
			tool := presentTool(interrupt.Approval.Tool)
			entry.Payload = map[string]any{"tool": tool}
			if interrupt.Approval.Risk != "" {
				entry.Payload["risk"] = interrupt.Approval.Risk
			}
			if interrupt.Approval.Reason != "" {
				entry.Payload["reason"] = interrupt.Approval.Reason
			}
		case transcript.QuestionInterrupt:
			if interrupt.Question == nil {
				continue
			}
			entry.Type = protocol.InterruptQuestion
			entry.Payload = map[string]any{"question": presentQuestion(*interrupt.Question)}
		default:
			continue
		}
		out = append(out, entry)
	}
	return out
}

func presentDelta(delta runs.ItemDelta) protocol.ItemDelta {
	kind := protocol.DeltaContent
	switch delta.Kind {
	case runs.ReasoningDeltaKind:
		kind = protocol.DeltaReasoning
	case runs.ToolArgumentsDelta:
		kind = protocol.DeltaToolArguments
	case runs.ToolOutputDelta:
		kind = protocol.DeltaToolOutput
	case runs.PlanDelta:
		kind = protocol.DeltaPlan
	}
	return protocol.ItemDelta{
		Type: kind, Index: delta.Index, Text: delta.Text,
		ArgumentsTextDelta: delta.ArgumentsTextDelta, Steps: presentPlanSteps(delta.Steps),
	}
}

func mapRunEvents(ctx context.Context, in <-chan runs.Event) <-chan protocol.RunEvent {
	out := make(chan protocol.RunEvent)
	go func() {
		defer close(out)
		for event := range in {
			wire := protocol.RunEvent{
				RunID: event.RunID, SegmentID: event.SegmentID,
				EventID: protocol.IDPrefixEvent + event.Seq, Timestamp: event.Timestamp,
				Event: presentRunEvent(event.Payload),
			}
			select {
			case out <- wire:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}
