package server

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
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
		Session:  artifactSessionFromDomain(portable.Session),
		Messages: messages, Runs: runs, Items: items, ToolResults: toolResults,
	}, nil
}

func artifactSessionFromDomain(value session.Session) protocol.ArtifactSession {
	return protocol.ArtifactSession{
		ID: value.ID, Title: value.Title, Cwd: value.Cwd, Model: value.Model,
		CreatedAt: value.StartedAt, UpdatedAt: value.UpdatedAt, Favorite: value.Favorite,
	}
}

// artifactToSession maps the durable wire session identity to the product
// session. Archive documents deliberately omit delegation lineage, so a
// restored artifact is always a standalone user-facing conversation.
func artifactToSession(value protocol.ArtifactSession) session.Session {
	return session.Session{
		ID:        value.ID,
		Title:     value.Title,
		Cwd:       value.Cwd,
		Model:     value.Model,
		StartedAt: value.CreatedAt,
		UpdatedAt: value.UpdatedAt,
		Favorite:  value.Favorite,
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
			out.Steps[index] = protocol.ArtifactPlanStep{ID: step.ID, Title: step.Title, Status: step.Status}
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

// portableArtifactFromWire performs only protocol decoding and enum mapping.
// Aggregate ownership, union rules, references, terminal boundaries, and tool
// result bindings are validated by sessions.RestorePortableSession.
func portableArtifactFromWire(art protocol.SessionArtifact) (sessions.PortableSnapshot, error) {
	if art.Session.ID == "" {
		return sessions.PortableSnapshot{}, invalidArtifact("artifact.session.id", "is required")
	}
	messages := make([]chat.Message, 0, len(art.Messages))
	for index, encoded := range art.Messages {
		var message chat.Message
		if err := json.Unmarshal(encoded, &message); err != nil {
			return sessions.PortableSnapshot{}, invalidArtifact(fmt.Sprintf("artifact.messages[%d]", index), "%v", err)
		}
		messages = append(messages, message)
	}
	runs := make([]sessions.PortableRun, 0, len(art.Runs))
	for index, encoded := range art.Runs {
		run, err := portableRunFromArtifact(fmt.Sprintf("artifact.runs[%d]", index), encoded)
		if err != nil {
			return sessions.PortableSnapshot{}, err
		}
		runs = append(runs, run)
	}
	items := make([]transcript.Item, 0, len(art.Items))
	for index, encoded := range art.Items {
		item, err := portableItemFromArtifact(art.Session.ID, fmt.Sprintf("artifact.items[%d]", index), encoded)
		if err != nil {
			return sessions.PortableSnapshot{}, err
		}
		items = append(items, item)
	}
	toolResults := make([]offload.ToolResultBlob, 0, len(art.ToolResults))
	for index, encoded := range art.ToolResults {
		id, err := offload.ParseID(encoded.ID)
		if err != nil {
			return sessions.PortableSnapshot{}, invalidArtifact(fmt.Sprintf("artifact.toolResults[%d].id", index), "%v", err)
		}
		toolResults = append(toolResults, offload.ToolResultBlob{
			ID: id, SessionID: art.Session.ID, ItemID: encoded.ItemID, ToolName: encoded.ToolName,
			Preview: encoded.Preview, Body: encoded.Body, CreatedAt: encoded.CreatedAt,
		})
	}
	return sessions.PortableSnapshot{
		Session: artifactToSession(art.Session), Messages: messages, Runs: runs, Items: items, ToolResults: toolResults,
	}, nil
}

func portableRunFromArtifact(path string, artifact protocol.ArtifactRun) (sessions.PortableRun, error) {
	outcome, err := portableOutcomeFromArtifact(path+".outcome.type", artifact.Outcome.Type)
	if err != nil {
		return sessions.PortableRun{}, err
	}
	result, err := portableRunResultFromArtifact(path+".outcome.result", artifact.Outcome.Result)
	if err != nil {
		return sessions.PortableRun{}, err
	}
	return sessions.PortableRun{
		SessionID: artifact.SessionID, ID: artifact.ID, SpawnedByItemID: artifact.SpawnedByItemID,
		Provider: artifact.Provider, Model: artifact.Model, Outcome: outcome, Result: result, Detail: artifact.Outcome.Detail,
		CreatedAt: artifact.CreatedAt, FinishedAt: artifact.FinishedAt,
		UpdatedAt: artifact.UpdatedAt, MessageMark: artifact.MessageMark,
	}, nil
}

func portableOutcomeFromArtifact(path, value string) (execution.Outcome, error) {
	switch value {
	case "completed":
		return execution.OutcomeCompleted, nil
	case "canceled":
		return execution.OutcomeCanceled, nil
	case "error":
		return execution.OutcomeError, nil
	case "maxBudget":
		return execution.OutcomeMaxBudget, nil
	case "maxSteps":
		return execution.OutcomeMaxSteps, nil
	default:
		return 0, invalidArtifact(path, "unknown value %q", value)
	}
}

func portableRunResultFromArtifact(path string, artifact *protocol.ArtifactRunResult) (*transcript.RunResult, error) {
	if artifact == nil {
		return nil, nil
	}
	usage := portableUsageFromArtifact(artifact.Usage)
	problem, err := portableProblemFromArtifact(path+".error", artifact.Error, transcript.RunProblem)
	if err != nil {
		return nil, err
	}
	return &transcript.RunResult{
		Usage: usage, Steps: artifact.Steps, Error: problem,
		Duration: time.Duration(artifact.DurationMs) * time.Millisecond,
	}, nil
}

func portableUsageFromArtifact(artifact *protocol.ArtifactUsage) *transcript.Usage {
	if artifact == nil {
		return nil
	}
	out := &transcript.Usage{ModelUsage: transcript.ModelUsage{
		InputTokens: artifact.InputTokens, OutputTokens: artifact.OutputTokens,
		CacheReadTokens: artifact.CacheReadTokens, CacheWriteTokens: artifact.CacheWriteTokens,
		ReasoningTokens: artifact.ReasoningTokens, CostUSD: artifact.CostUSD,
	}}
	if len(artifact.ByModel) != 0 {
		out.ByModel = make(map[string]transcript.ModelUsage, len(artifact.ByModel))
		for model, values := range artifact.ByModel {
			out.ByModel[model] = transcript.ModelUsage{
				InputTokens: values.InputTokens, OutputTokens: values.OutputTokens,
				CacheReadTokens: values.CacheReadTokens, CacheWriteTokens: values.CacheWriteTokens,
				ReasoningTokens: values.ReasoningTokens, CostUSD: values.CostUSD,
			}
		}
	}
	return out
}

func portableItemFromArtifact(sessionID, path string, artifact protocol.ArtifactItem) (transcript.Item, error) {
	status, err := portableItemStatus(path+".status", artifact.Status)
	if err != nil {
		return transcript.Item{}, err
	}
	kind, err := portableItemKind(path+".type", artifact.Type)
	if err != nil {
		return transcript.Item{}, err
	}
	problem, err := portableProblemFromArtifact(path+".error", artifact.Error, transcript.ToolProblem)
	if err != nil {
		return transcript.Item{}, err
	}
	out := transcript.Item{
		SessionID: sessionID, ID: artifact.ID, RunID: artifact.RunID, Status: status, Kind: kind,
		CreatedAt: artifact.CreatedAt, Text: artifact.Text, Redacted: artifact.Redacted,
		SafetyClass: tool.SafetyClass(artifact.SafetyClass), Error: problem,
		Summary: artifact.Summary, DroppedMessages: artifact.DroppedMessages,
	}
	if len(artifact.Content) != 0 {
		out.Content = make([]transcript.ContentBlock, len(artifact.Content))
		for index, block := range artifact.Content {
			content, err := portableContentFromArtifact(fmt.Sprintf("%s.content[%d]", path, index), block)
			if err != nil {
				return transcript.Item{}, err
			}
			out.Content[index] = content
		}
	}
	if len(artifact.Steps) != 0 {
		out.Steps = make([]transcript.PlanStep, len(artifact.Steps))
		for index, step := range artifact.Steps {
			out.Steps[index] = transcript.PlanStep{ID: step.ID, Title: step.Title, Status: step.Status}
		}
	}
	if artifact.Question != nil {
		question, err := portableQuestionFromArtifact(path+".question", *artifact.Question)
		if err != nil {
			return transcript.Item{}, err
		}
		out.Question = &question
	}
	if artifact.Tool != nil {
		invocation, err := portableToolFromArtifact(path+".tool", *artifact.Tool)
		if err != nil {
			return transcript.Item{}, err
		}
		out.Tool = &invocation
	}
	return out, nil
}

func portableItemStatus(path, value string) (transcript.ItemStatus, error) {
	switch value {
	case "running":
		return transcript.ItemRunning, nil
	case "completed":
		return transcript.ItemCompleted, nil
	case "incomplete":
		return transcript.ItemIncomplete, nil
	default:
		return 0, invalidArtifact(path, "unknown value %q", value)
	}
}

func portableItemKind(path, value string) (transcript.ItemKind, error) {
	switch value {
	case "userMessage":
		return transcript.UserMessage, nil
	case "agentMessage":
		return transcript.AgentMessage, nil
	case "reasoning":
		return transcript.Reasoning, nil
	case "plan":
		return transcript.Plan, nil
	case "question":
		return transcript.QuestionItem, nil
	case "toolCall":
		return transcript.ToolCall, nil
	case "compaction":
		return transcript.Compaction, nil
	default:
		return 0, invalidArtifact(path, "unknown value %q", value)
	}
}

func portableContentFromArtifact(path string, artifact protocol.ArtifactContentBlock) (transcript.ContentBlock, error) {
	switch artifact.Type {
	case "text":
		return transcript.ContentBlock{Kind: transcript.TextContent, Text: artifact.Text, Mime: artifact.Mime, Data: artifact.Data}, nil
	case "image":
		return transcript.ContentBlock{Kind: transcript.ImageContent, Text: artifact.Text, Mime: artifact.Mime, Data: artifact.Data}, nil
	default:
		return transcript.ContentBlock{}, invalidArtifact(path+".type", "unknown value %q", artifact.Type)
	}
}

func portableQuestionFromArtifact(path string, artifact protocol.ArtifactQuestion) (transcript.Question, error) {
	fields := make([]transcript.QuestionField, len(artifact.Fields))
	for index, field := range artifact.Fields {
		kind, err := portableQuestionFieldKind(fmt.Sprintf("%s.fields[%d].type", path, index), field.Type)
		if err != nil {
			return transcript.Question{}, err
		}
		options := make([]transcript.QuestionOption, len(field.Options))
		for optionIndex, option := range field.Options {
			options[optionIndex] = transcript.QuestionOption{Label: option.Label, Description: option.Description, Preview: option.Preview}
		}
		fields[index] = transcript.QuestionField{
			Name: field.Name, Label: field.Label, Header: field.Header, Required: field.Required,
			Kind: kind, Options: options, Multiple: field.Multiple,
		}
	}
	return transcript.Question{Prompt: artifact.Prompt, Fields: fields}, nil
}

func portableQuestionFieldKind(path, value string) (transcript.QuestionFieldKind, error) {
	switch value {
	case "text":
		return transcript.QuestionText, nil
	case "choice":
		return transcript.QuestionChoice, nil
	default:
		return 0, invalidArtifact(path, "unknown value %q", value)
	}
}

func portableToolFromArtifact(path string, artifact protocol.ArtifactToolInvocation) (transcript.ToolInvocation, error) {
	arguments, err := tool.ArgumentsFromMap(artifact.Arguments)
	if err != nil {
		return transcript.ToolInvocation{}, invalidArtifact(path+".arguments", "%v", err)
	}
	var result *tool.Result
	if artifact.Result != nil {
		value, err := tool.NewResult(artifact.Result)
		if err != nil {
			return transcript.ToolInvocation{}, invalidArtifact(path+".result", "%v", err)
		}
		result = &value
	}
	return transcript.ToolInvocation{Name: artifact.Name, Arguments: arguments, Result: result}, nil
}

func portableProblemFromArtifact(path string, artifact *protocol.ArtifactProblem, scope transcript.ProblemScope) (*transcript.Problem, error) {
	if artifact == nil {
		return nil, nil
	}
	kind, err := portableProblemKind(path+".type", artifact.Type)
	if err != nil {
		return nil, err
	}
	return &transcript.Problem{
		Kind: kind, Scope: scope, Detail: artifact.Detail, DocURL: artifact.DocURL,
		Retryable: artifact.Retryable, RetryAfterSeconds: artifact.RetryAfterSeconds,
	}, nil
}

func portableProblemKind(path, value string) (transcript.ProblemKind, error) {
	switch value {
	case "internalError":
		return transcript.InternalProblem, nil
	case "runLost":
		return transcript.RunLostProblem, nil
	case "agentStuck":
		return transcript.AgentStuckProblem, nil
	case "rateLimited":
		return transcript.RateLimitedProblem, nil
	case "invalidApiKey":
		return transcript.InvalidAPIKeyProblem, nil
	case "timeout":
		return transcript.TimeoutProblem, nil
	case "providerUnavailable":
		return transcript.ProviderUnavailableProblem, nil
	case "providerRejected":
		return transcript.ProviderRejectedProblem, nil
	case "deniedByUser":
		return transcript.DeniedByUserProblem, nil
	case "toolFailed":
		return transcript.ToolFailedProblem, nil
	default:
		return 0, invalidArtifact(path, "unknown value %q", value)
	}
}
