package server

import (
	"encoding/json"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
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
		encoded, err := artifactRunFromPortable(run)
		if err != nil {
			return protocol.SessionArtifact{}, err
		}
		runs = append(runs, encoded)
	}
	items := make([]protocol.ArtifactItem, 0, len(portable.Items))
	for _, item := range portable.Items {
		encoded, err := artifactItemFromTranscript(item)
		if err != nil {
			return protocol.SessionArtifact{}, err
		}
		items = append(items, encoded)
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

func artifactRunFromPortable(run sessions.PortableRun) (protocol.ArtifactRun, error) {
	outcome, err := artifactOutcomeType(run.Outcome)
	if err != nil {
		return protocol.ArtifactRun{}, fmt.Errorf("run %q outcome: %w", run.ID, err)
	}
	result, err := artifactRunResultFromDomain(run.Result)
	if err != nil {
		return protocol.ArtifactRun{}, fmt.Errorf("run %q result: %w", run.ID, err)
	}
	return protocol.ArtifactRun{
		ID: run.ID, SessionID: run.SessionID, SpawnedByItemID: run.SpawnedByItemID,
		Provider: run.Provider, Model: run.Model,
		Outcome: protocol.ArtifactOutcome{
			Type: outcome, Result: result, Detail: run.Detail,
		},
		CreatedAt: run.CreatedAt, FinishedAt: run.FinishedAt,
		UpdatedAt: run.UpdatedAt, MessageMark: run.MessageMark,
	}, nil
}

func artifactOutcomeType(outcome execution.Outcome) (protocol.ArtifactOutcomeType, error) {
	switch outcome {
	case execution.OutcomeCompleted:
		return protocol.ArtifactOutcomeCompleted, nil
	case execution.OutcomeCanceled:
		return protocol.ArtifactOutcomeCanceled, nil
	case execution.OutcomeError:
		return protocol.ArtifactOutcomeError, nil
	case execution.OutcomeMaxBudget:
		return protocol.ArtifactOutcomeMaxBudget, nil
	case execution.OutcomeMaxSteps:
		return protocol.ArtifactOutcomeMaxSteps, nil
	default:
		return "", fmt.Errorf("unknown value %d", outcome)
	}
}

func artifactRunResultFromDomain(result *transcript.RunResult) (*protocol.ArtifactRunResult, error) {
	if result == nil {
		return nil, nil
	}
	problem, err := artifactProblemFromDomain(result.Error)
	if err != nil {
		return nil, err
	}
	return &protocol.ArtifactRunResult{
		Usage: artifactUsageFromDomain(result.Usage), Steps: result.Steps,
		Error: problem, DurationMs: int(result.Duration.Milliseconds()),
	}, nil
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

func artifactProblemFromDomain(problem *transcript.Problem) (*protocol.ArtifactProblem, error) {
	if problem == nil {
		return nil, nil
	}
	kind, err := artifactProblemType(problem.Kind)
	if err != nil {
		return nil, err
	}
	return &protocol.ArtifactProblem{
		Type: kind, Detail: problem.Detail, DocURL: problem.DocURL,
		Retryable: problem.Retryable, RetryAfterSeconds: problem.RetryAfterSeconds,
	}, nil
}

func artifactProblemType(kind transcript.ProblemKind) (protocol.ArtifactProblemType, error) {
	switch kind {
	case transcript.InternalProblem:
		return protocol.ArtifactProblemInternalError, nil
	case transcript.RunLostProblem:
		return protocol.ArtifactProblemRunLost, nil
	case transcript.AgentStuckProblem:
		return protocol.ArtifactProblemAgentStuck, nil
	case transcript.RateLimitedProblem:
		return protocol.ArtifactProblemRateLimited, nil
	case transcript.InvalidAPIKeyProblem:
		return protocol.ArtifactProblemInvalidAPIKey, nil
	case transcript.TimeoutProblem:
		return protocol.ArtifactProblemTimeout, nil
	case transcript.ProviderUnavailableProblem:
		return protocol.ArtifactProblemProviderUnavailable, nil
	case transcript.ProviderRejectedProblem:
		return protocol.ArtifactProblemProviderRejected, nil
	case transcript.DeniedByUserProblem:
		return protocol.ArtifactProblemDeniedByUser, nil
	case transcript.ToolFailedProblem:
		return protocol.ArtifactProblemToolFailed, nil
	default:
		return "", fmt.Errorf("unknown value %d", kind)
	}
}

func artifactItemFromTranscript(item transcript.Item) (protocol.ArtifactItem, error) {
	status, err := artifactItemStatus(item.Status)
	if err != nil {
		return protocol.ArtifactItem{}, fmt.Errorf("item %q status: %w", item.ID, err)
	}
	kind, err := artifactItemType(item.Kind)
	if err != nil {
		return protocol.ArtifactItem{}, fmt.Errorf("item %q type: %w", item.ID, err)
	}
	problem, err := artifactProblemFromDomain(item.Error)
	if err != nil {
		return protocol.ArtifactItem{}, fmt.Errorf("item %q error: %w", item.ID, err)
	}
	safetyClass, err := artifactSafetyClass(item.SafetyClass)
	if err != nil {
		return protocol.ArtifactItem{}, fmt.Errorf("item %q safety class: %w", item.ID, err)
	}
	out := protocol.ArtifactItem{
		ID: item.ID, RunID: item.RunID, Status: status, CreatedAt: item.CreatedAt,
		Type: kind, Text: item.Text, Redacted: item.Redacted,
		SafetyClass: safetyClass, Error: problem,
		Summary: item.Summary, DroppedMessages: item.DroppedMessages,
	}
	if len(item.Content) != 0 {
		out.Content = make([]protocol.ArtifactContentBlock, len(item.Content))
		for index, block := range item.Content {
			contentType, err := artifactContentType(block.Kind)
			if err != nil {
				return protocol.ArtifactItem{}, fmt.Errorf("item %q content %d: %w", item.ID, index, err)
			}
			out.Content[index] = protocol.ArtifactContentBlock{Type: contentType, Text: block.Text, Mime: block.Mime, Data: block.Data}
		}
	}
	if len(item.Steps) != 0 {
		out.Steps = make([]protocol.ArtifactPlanStep, len(item.Steps))
		for index, step := range item.Steps {
			stepStatus, err := artifactPlanStepStatus(step.Status)
			if err != nil {
				return protocol.ArtifactItem{}, fmt.Errorf("item %q plan step %d: %w", item.ID, index, err)
			}
			out.Steps[index] = protocol.ArtifactPlanStep{ID: step.ID, Title: step.Title, Status: stepStatus}
		}
	}
	if item.Question != nil {
		question, err := artifactQuestionFromDomain(*item.Question)
		if err != nil {
			return protocol.ArtifactItem{}, fmt.Errorf("item %q question: %w", item.ID, err)
		}
		out.Question = question
	}
	if item.Tool != nil {
		tool := protocol.ArtifactToolInvocation{Name: item.Tool.Name, Arguments: item.Tool.Arguments.Map()}
		if item.Tool.Result != nil {
			tool.Result = item.Tool.Result.Any()
		}
		out.Tool = &tool
	}
	return out, nil
}

func artifactItemStatus(status transcript.ItemStatus) (protocol.ItemStatus, error) {
	switch status {
	case transcript.ItemRunning:
		return protocol.ItemStatusRunning, nil
	case transcript.ItemCompleted:
		return protocol.ItemStatusCompleted, nil
	case transcript.ItemIncomplete:
		return protocol.ItemStatusIncomplete, nil
	default:
		return "", fmt.Errorf("unknown value %d", status)
	}
}

func artifactItemType(kind transcript.ItemKind) (protocol.ItemType, error) {
	switch kind {
	case transcript.UserMessage:
		return protocol.ItemTypeUserMessage, nil
	case transcript.AgentMessage:
		return protocol.ItemTypeAgentMessage, nil
	case transcript.Reasoning:
		return protocol.ItemTypeReasoning, nil
	case transcript.Plan:
		return protocol.ItemTypePlan, nil
	case transcript.QuestionItem:
		return protocol.ItemTypeQuestion, nil
	case transcript.ToolCall:
		return protocol.ItemTypeToolCall, nil
	case transcript.Compaction:
		return protocol.ItemTypeCompaction, nil
	default:
		return "", fmt.Errorf("unknown value %d", kind)
	}
}

func artifactContentType(kind transcript.ContentKind) (protocol.ContentBlockType, error) {
	switch kind {
	case transcript.TextContent:
		return protocol.ContentBlockText, nil
	case transcript.ImageContent:
		return protocol.ContentBlockImage, nil
	default:
		return "", fmt.Errorf("unknown value %d", kind)
	}
}

func artifactQuestionFromDomain(question transcript.Question) (*protocol.ArtifactQuestion, error) {
	fields := make([]protocol.ArtifactQuestionField, len(question.Fields))
	for index, field := range question.Fields {
		options := make([]protocol.ArtifactQuestionOption, len(field.Options))
		for optionIndex, option := range field.Options {
			options[optionIndex] = protocol.ArtifactQuestionOption{Label: option.Label, Description: option.Description, Preview: option.Preview}
		}
		var fieldType protocol.QuestionFieldType
		switch field.Kind {
		case transcript.QuestionText:
			fieldType = protocol.QuestionFieldText
		case transcript.QuestionChoice:
			fieldType = protocol.QuestionFieldChoice
		default:
			return nil, fmt.Errorf("field %d has unknown type %d", index, field.Kind)
		}
		fields[index] = protocol.ArtifactQuestionField{
			Name: field.Name, Label: field.Label, Header: field.Header, Required: field.Required,
			Type: fieldType, Options: options, Multiple: field.Multiple,
		}
	}
	return &protocol.ArtifactQuestion{Prompt: question.Prompt, Fields: fields}, nil
}

func artifactSafetyClass(class tool.SafetyClass) (protocol.SafetyClass, error) {
	switch class {
	case "":
		return "", nil
	case tool.SafetyClassSafe:
		return protocol.SafetyClassSafe, nil
	case tool.SafetyClassWrite:
		return protocol.SafetyClassWrite, nil
	case tool.SafetyClassExec:
		return protocol.SafetyClassExec, nil
	case tool.SafetyClassNetwork:
		return protocol.SafetyClassNetwork, nil
	default:
		return "", fmt.Errorf("unknown value %q", class)
	}
}

func artifactPlanStepStatus(status transcript.PlanStepStatus) (protocol.PlanStepStatus, error) {
	switch status {
	case transcript.PlanStepPending:
		return protocol.PlanStepPending, nil
	case transcript.PlanStepRunning:
		return protocol.PlanStepRunning, nil
	case transcript.PlanStepCompleted:
		return protocol.PlanStepCompleted, nil
	case transcript.PlanStepFailed:
		return protocol.PlanStepFailed, nil
	default:
		return "", fmt.Errorf("unknown value %q", status)
	}
}
