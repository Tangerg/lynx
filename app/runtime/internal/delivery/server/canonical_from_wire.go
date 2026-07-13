package server

import (
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

func canonicalRunFromWire(sessionID string, ref protocol.RunRef, updatedAt time.Time, messageMark int) transcript.Run {
	run := transcript.Run{
		SessionID: sessionID, ID: ref.ID, SpawnedByItemID: ref.SpawnedByItemID,
		Provider: ref.Provider, Model: ref.Model, State: execution.Running,
		CreatedAt: ref.CreatedAt, FinishedAt: ref.FinishedAt,
		UpdatedAt: updatedAt, MessageMark: messageMark,
	}
	if ref.Status != protocol.RunStatusFinished || ref.Outcome == nil {
		return run
	}
	if ref.Outcome.Type == protocol.OutcomeInterrupt {
		run.State = execution.Interrupted
		run.Interrupts = canonicalInterruptsFromWire(ref.Outcome.Interrupts)
		return run
	}
	outcome := canonicalOutcome(ref.Outcome.Type)
	state, _ := execution.Running.Terminate(outcome)
	run.State = state
	run.Outcome = &outcome
	run.Result = canonicalRunResult(ref.Outcome.Result)
	run.Detail = ref.Outcome.Detail
	return run
}

func canonicalOutcome(kind protocol.RunOutcomeType) execution.Outcome {
	switch kind {
	case protocol.OutcomeCanceled:
		return execution.OutcomeCanceled
	case protocol.OutcomeError:
		return execution.OutcomeError
	case protocol.OutcomeMaxBudget:
		return execution.OutcomeMaxBudget
	case protocol.OutcomeMaxSteps:
		return execution.OutcomeMaxSteps
	default:
		return execution.OutcomeCompleted
	}
}

func canonicalRunResult(result *protocol.RunResult) *transcript.RunResult {
	if result == nil {
		return nil
	}
	steps := 0
	if result.Steps != nil {
		steps = *result.Steps
	}
	return &transcript.RunResult{
		Usage: canonicalUsage(result.Usage), Steps: steps,
		Error: canonicalProblem(result.Error), Duration: time.Duration(result.DurationMs) * time.Millisecond,
	}
}

func canonicalUsage(usage *protocol.Usage) *transcript.Usage {
	if usage == nil {
		return nil
	}
	out := &transcript.Usage{ModelUsage: canonicalModelUsage(usage.ModelUsage)}
	if len(usage.ByModel) > 0 {
		out.ByModel = make(map[string]transcript.ModelUsage, len(usage.ByModel))
		for model, modelUsage := range usage.ByModel {
			out.ByModel[model] = canonicalModelUsage(modelUsage)
		}
	}
	return out
}

func canonicalModelUsage(usage protocol.ModelUsage) transcript.ModelUsage {
	return transcript.ModelUsage{
		InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens,
		CacheReadTokens: usage.CacheReadTokens, CacheWriteTokens: usage.CacheWriteTokens,
		ReasoningTokens: usage.ReasoningTokens, CostUSD: usage.CostUSD,
	}
}

func canonicalItemFromWire(sessionID string, item protocol.Item) transcript.Item {
	out := transcript.Item{
		SessionID: sessionID, ID: item.ID, RunID: item.RunID,
		Status: canonicalItemStatus(item.Status), Kind: canonicalItemKind(item.Type),
		CreatedAt: item.CreatedAt, Text: item.Text, Redacted: item.Redacted,
		SafetyClass: string(item.SafetyClass), Error: canonicalProblem(item.Error),
		Summary: item.Summary, DroppedMessages: item.DroppedMessages,
	}
	if len(item.Content) > 0 {
		out.Content = make([]transcript.ContentBlock, len(item.Content))
		for i, block := range item.Content {
			out.Content[i] = canonicalContent(block)
		}
	}
	if len(item.Steps) > 0 {
		out.Steps = make([]transcript.PlanStep, len(item.Steps))
		for i, step := range item.Steps {
			out.Steps[i] = transcript.PlanStep{ID: step.ID, Title: step.Title, Status: string(step.Status)}
		}
	}
	if item.Question != nil {
		question := canonicalQuestion(*item.Question)
		out.Question = &question
	}
	if item.Tool != nil {
		out.Tool = &transcript.ToolInvocation{
			Name: item.Tool.Name, Arguments: item.Tool.Arguments,
			Result: &transcript.ToolResult{Kind: transcript.RawToolResult, Raw: item.Tool.Result},
		}
		if item.Tool.Result == nil {
			out.Tool.Result = nil
		}
	}
	return out
}

func canonicalItemStatus(status protocol.ItemStatus) transcript.ItemStatus {
	switch status {
	case protocol.ItemStatusCompleted:
		return transcript.ItemCompleted
	case protocol.ItemStatusIncomplete:
		return transcript.ItemIncomplete
	default:
		return transcript.ItemRunning
	}
}

func canonicalItemKind(kind protocol.ItemType) transcript.ItemKind {
	switch kind {
	case protocol.ItemTypeAgentMessage:
		return transcript.AgentMessage
	case protocol.ItemTypeReasoning:
		return transcript.Reasoning
	case protocol.ItemTypePlan:
		return transcript.Plan
	case protocol.ItemTypeQuestion:
		return transcript.QuestionItem
	case protocol.ItemTypeToolCall:
		return transcript.ToolCall
	case protocol.ItemTypeCompaction:
		return transcript.Compaction
	default:
		return transcript.UserMessage
	}
}

func canonicalContent(block protocol.ContentBlock) transcript.ContentBlock {
	kind := transcript.TextContent
	if block.Type == protocol.ContentBlockImage {
		kind = transcript.ImageContent
	}
	return transcript.ContentBlock{Kind: kind, Text: block.Text, Mime: block.Mime, Data: block.Data}
}

func canonicalQuestion(question protocol.Question) transcript.Question {
	fields := make([]transcript.QuestionField, len(question.Fields))
	for i, field := range question.Fields {
		kind := transcript.QuestionText
		if field.Type == protocol.QuestionFieldChoice {
			kind = transcript.QuestionChoice
		}
		options := make([]transcript.QuestionOption, len(field.Options))
		for j, option := range field.Options {
			options[j] = transcript.QuestionOption{
				Label: option.Label, Description: option.Description, Preview: option.Preview,
			}
		}
		fields[i] = transcript.QuestionField{
			Name: field.Name, Label: field.Label, Header: field.Header,
			Required: field.Required, Kind: kind, Options: options, Multiple: field.Multiple,
		}
	}
	return transcript.Question{Prompt: question.Prompt, Fields: fields}
}

func canonicalProblem(problem *protocol.ProblemData) *transcript.Problem {
	if problem == nil {
		return nil
	}
	kind := transcript.InternalProblem
	switch problem.Type {
	case protocol.ProblemAgentStuck:
		kind = transcript.AgentStuckProblem
	case protocol.ProblemRateLimited:
		kind = transcript.RateLimitedProblem
	case protocol.ProblemInvalidAPIKey:
		kind = transcript.InvalidAPIKeyProblem
	case protocol.ProblemTimeout:
		kind = transcript.TimeoutProblem
	case protocol.ProblemProviderUnavailable:
		kind = transcript.ProviderUnavailableProblem
	case protocol.ProblemProviderRejected:
		kind = transcript.ProviderRejectedProblem
	case protocol.ProblemDeniedByUser:
		kind = transcript.DeniedByUserProblem
	case protocol.ProblemToolFailed:
		kind = transcript.ToolFailedProblem
	}
	scope := transcript.RunProblem
	if problem.Channel == protocol.ErrorChannelTool {
		scope = transcript.ToolProblem
	}
	return &transcript.Problem{
		Kind: kind, Scope: scope, Detail: problem.Detail, DocURL: problem.DocURL,
		Retryable: problem.Retryable, RetryAfterSeconds: problem.RetryAfterSeconds,
	}
}

func canonicalInterruptsFromWire(entries []protocol.Interrupt) []transcript.Interrupt {
	out := make([]transcript.Interrupt, 0, len(entries))
	for _, entry := range entries {
		interrupt := transcript.Interrupt{ItemID: entry.ItemID}
		switch entry.Type {
		case protocol.InterruptApproval:
			tool, ok := toolFromPayload(entry.Payload["tool"])
			if !ok {
				continue
			}
			interrupt.Kind = transcript.ApprovalInterrupt
			interrupt.Approval = &transcript.Approval{
				Tool: tool, Risk: stringFromMap(entry.Payload, "risk"), Reason: stringFromMap(entry.Payload, "reason"),
			}
		case protocol.InterruptQuestion:
			question, ok := questionFromPayload(entry.Payload["question"])
			if !ok {
				continue
			}
			interrupt.Kind = transcript.QuestionInterrupt
			interrupt.Question = &question
		default:
			continue
		}
		out = append(out, interrupt)
	}
	return out
}

func toolFromPayload(value any) (transcript.ToolInvocation, bool) {
	switch tool := value.(type) {
	case protocol.ToolInvocation:
		return transcript.ToolInvocation{Name: tool.Name, Arguments: tool.Arguments}, tool.Name != ""
	case map[string]any:
		name, _ := tool["name"].(string)
		arguments, _ := tool["arguments"].(map[string]any)
		return transcript.ToolInvocation{Name: name, Arguments: arguments}, name != ""
	default:
		return transcript.ToolInvocation{}, false
	}
}

func questionFromPayload(value any) (transcript.Question, bool) {
	switch question := value.(type) {
	case protocol.Question:
		return canonicalQuestion(question), true
	case map[string]any:
		prompt, _ := question["prompt"].(string)
		rawFields, _ := question["fields"].([]any)
		fields := make([]transcript.QuestionField, 0, len(rawFields))
		for _, raw := range rawFields {
			fieldMap, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			field := transcript.QuestionField{
				Name: stringFromMap(fieldMap, "name"), Label: stringFromMap(fieldMap, "label"),
				Header: stringFromMap(fieldMap, "header"), Required: boolFromMap(fieldMap, "required"),
				Multiple: boolFromMap(fieldMap, "multiple"),
			}
			if stringFromMap(fieldMap, "type") == string(protocol.QuestionFieldChoice) {
				field.Kind = transcript.QuestionChoice
			}
			rawOptions, _ := fieldMap["options"].([]any)
			field.Options = make([]transcript.QuestionOption, 0, len(rawOptions))
			for _, rawOption := range rawOptions {
				optionMap, ok := rawOption.(map[string]any)
				if !ok {
					continue
				}
				field.Options = append(field.Options, transcript.QuestionOption{
					Label: stringFromMap(optionMap, "label"), Description: stringFromMap(optionMap, "description"),
					Preview: stringFromMap(optionMap, "preview"),
				})
			}
			fields = append(fields, field)
		}
		return transcript.Question{Prompt: prompt, Fields: fields}, true
	default:
		return transcript.Question{}, false
	}
}

func stringFromMap(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return value
}

func boolFromMap(values map[string]any, key string) bool {
	value, _ := values[key].(bool)
	return value
}
