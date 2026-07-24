package server

import (
	"encoding/json"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

// artifactFromPortable maps Application's terminal archive projection to the
// versioned protocol document. It deliberately uses canonical domain values,
// never the live client presentation (which may reshape a known tool result).
func artifactFromPortable(portable sessions.PortableSnapshot) (protocol.SessionArtifact, error) {
	messages := make([]json.RawMessage, 0, len(portable.Messages))
	for _, message := range portable.Messages {
		encoded, err := json.Marshal(message)
		if err != nil {
			return protocol.SessionArtifact{}, fmt.Errorf("marshal message: %w", err)
		}
		messages = append(messages, encoded)
	}

	runs := make([]protocol.ArtifactRun, 0, len(portable.Runs))
	for _, run := range portable.Runs {
		runs = append(runs, artifactRunFromPortable(run))
	}
	items := make([]protocol.ArtifactItem, 0, len(portable.Items))
	for _, item := range portable.Items {
		items = append(items, artifactItemFromTranscript(item))
	}
	toolResults := make([]protocol.ArtifactToolResult, 0, len(portable.ToolResults))
	for _, blob := range portable.ToolResults {
		toolResults = append(toolResults, protocol.ArtifactToolResult{
			ID: blob.ID.String(), ItemID: blob.ItemID, ToolName: blob.ToolName,
			Preview: blob.Preview, Body: blob.Body, CreatedAt: blob.CreatedAt,
		})
	}
	return protocol.SessionArtifact{
		Version:  protocol.SessionArtifactVersion,
		Session:  artifactSessionFromPortable(portable.Session),
		Messages: messages, Runs: runs, Items: items, ToolResults: toolResults,
	}, nil
}

func artifactSessionFromPortable(value sessions.PortableSession) protocol.ArtifactSession {
	return protocol.ArtifactSession{
		ID: value.ID, Title: value.Title, Cwd: value.Cwd, Model: value.Model,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt, Favorite: value.Favorite,
	}
}

func artifactRunFromPortable(run sessions.PortableRun) protocol.ArtifactRun {
	return protocol.ArtifactRun{
		ID: run.ID, SessionID: run.SessionID, SpawnedByItemID: run.SpawnedByItemID,
		Provider: run.Provider, Model: run.Model,
		Outcome: protocol.ArtifactOutcome{
			Type: artifactOutcomeType(run.Outcome), Result: artifactRunResultFromDomain(run.Result), Detail: run.Detail,
		},
		CreatedAt: run.CreatedAt, FinishedAt: run.FinishedAt,
		UpdatedAt: run.UpdatedAt, MessageMark: run.MessageMark,
	}
}

func artifactOutcomeType(outcome execution.Outcome) string {
	switch outcome {
	case execution.OutcomeCanceled:
		return "canceled"
	case execution.OutcomeError:
		return "error"
	case execution.OutcomeMaxBudget:
		return "maxBudget"
	case execution.OutcomeMaxSteps:
		return "maxSteps"
	default:
		return "completed"
	}
}

func artifactRunResultFromDomain(result *transcript.RunResult) *protocol.ArtifactRunResult {
	if result == nil {
		return nil
	}
	return &protocol.ArtifactRunResult{
		Usage: artifactUsageFromDomain(result.Usage), Steps: result.Steps,
		Error: artifactProblemFromDomain(result.Error), DurationMs: int(result.Duration.Milliseconds()),
	}
}

func artifactUsageFromDomain(usage *transcript.Usage) *protocol.ArtifactUsage {
	if usage == nil {
		return nil
	}
	out := &protocol.ArtifactUsage{
		InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens,
		CacheReadTokens: usage.CacheReadTokens, CacheWriteTokens: usage.CacheWriteTokens,
		ReasoningTokens: usage.ReasoningTokens, CostUSD: usage.CostUSD,
	}
	if len(usage.ByModel) != 0 {
		out.ByModel = make(map[string]protocol.ArtifactModelUsage, len(usage.ByModel))
		for model, values := range usage.ByModel {
			out.ByModel[model] = protocol.ArtifactModelUsage{
				InputTokens: values.InputTokens, OutputTokens: values.OutputTokens,
				CacheReadTokens: values.CacheReadTokens, CacheWriteTokens: values.CacheWriteTokens,
				ReasoningTokens: values.ReasoningTokens, CostUSD: values.CostUSD,
			}
		}
	}
	return out
}

func artifactProblemFromDomain(problem *transcript.Problem) *protocol.ArtifactProblem {
	if problem == nil {
		return nil
	}
	return &protocol.ArtifactProblem{
		Type: artifactProblemType(problem.Kind), Detail: problem.Detail, DocURL: problem.DocURL,
		Retryable: problem.Retryable, RetryAfterSeconds: problem.RetryAfterSeconds,
	}
}

func artifactProblemType(kind transcript.ProblemKind) string {
	switch kind {
	case transcript.RunLostProblem:
		return "runLost"
	case transcript.AgentStuckProblem:
		return "agentStuck"
	case transcript.RateLimitedProblem:
		return "rateLimited"
	case transcript.InvalidAPIKeyProblem:
		return "invalidApiKey"
	case transcript.TimeoutProblem:
		return "timeout"
	case transcript.ProviderUnavailableProblem:
		return "providerUnavailable"
	case transcript.ProviderRejectedProblem:
		return "providerRejected"
	case transcript.DeniedByUserProblem:
		return "deniedByUser"
	case transcript.ToolFailedProblem:
		return "toolFailed"
	default:
		return "internalError"
	}
}

func artifactItemFromTranscript(item transcript.Item) protocol.ArtifactItem {
	out := protocol.ArtifactItem{
		ID: item.ID, RunID: item.RunID, Status: artifactItemStatus(item.Status), CreatedAt: item.CreatedAt,
		Type: artifactItemType(item.Kind), Text: item.Text, Redacted: item.Redacted,
		SafetyClass: string(item.SafetyClass), Error: artifactProblemFromDomain(item.Error),
		Summary: item.Summary, DroppedMessages: item.DroppedMessages,
	}
	if len(item.Content) != 0 {
		out.Content = make([]protocol.ArtifactContentBlock, len(item.Content))
		for index, block := range item.Content {
			out.Content[index] = protocol.ArtifactContentBlock{Type: artifactContentType(block.Kind), Text: block.Text, Mime: block.Mime, Data: block.Data}
		}
	}
	if len(item.Steps) != 0 {
		out.Steps = make([]protocol.ArtifactPlanStep, len(item.Steps))
		for index, step := range item.Steps {
			out.Steps[index] = protocol.ArtifactPlanStep{ID: step.ID, Title: step.Title, Status: string(step.Status)}
		}
	}
	if item.Question != nil {
		out.Question = artifactQuestionFromDomain(*item.Question)
	}
	if item.Tool != nil {
		tool := protocol.ArtifactToolInvocation{Name: item.Tool.Name, Arguments: item.Tool.Arguments.Map()}
		if item.Tool.Result != nil {
			tool.Result = item.Tool.Result.Any()
		}
		out.Tool = &tool
	}
	return out
}

func artifactItemStatus(status transcript.ItemStatus) string {
	switch status {
	case transcript.ItemCompleted:
		return "completed"
	case transcript.ItemIncomplete:
		return "incomplete"
	default:
		return "running"
	}
}

func artifactItemType(kind transcript.ItemKind) string {
	switch kind {
	case transcript.AgentMessage:
		return "agentMessage"
	case transcript.Reasoning:
		return "reasoning"
	case transcript.Plan:
		return "plan"
	case transcript.QuestionItem:
		return "question"
	case transcript.ToolCall:
		return "toolCall"
	case transcript.Compaction:
		return "compaction"
	default:
		return "userMessage"
	}
}

func artifactContentType(kind transcript.ContentKind) string {
	if kind == transcript.ImageContent {
		return "image"
	}
	return "text"
}

func artifactQuestionFromDomain(question transcript.Question) *protocol.ArtifactQuestion {
	fields := make([]protocol.ArtifactQuestionField, len(question.Fields))
	for index, field := range question.Fields {
		options := make([]protocol.ArtifactQuestionOption, len(field.Options))
		for optionIndex, option := range field.Options {
			options[optionIndex] = protocol.ArtifactQuestionOption{Label: option.Label, Description: option.Description, Preview: option.Preview}
		}
		fieldType := "text"
		if field.Kind == transcript.QuestionChoice {
			fieldType = "choice"
		}
		fields[index] = protocol.ArtifactQuestionField{
			Name: field.Name, Label: field.Label, Header: field.Header, Required: field.Required,
			Type: fieldType, Options: options, Multiple: field.Multiple,
		}
	}
	return &protocol.ArtifactQuestion{Prompt: question.Prompt, Fields: fields}
}
