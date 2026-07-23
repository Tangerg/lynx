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
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// invalidArtifact is the protocol adapter's structural-document error. Semantic
// aggregate validation is deliberately performed by sessions.RestorePortableSession.
func invalidArtifact(path, format string, args ...any) error {
	detail := fmt.Sprintf(format, args...)
	return fmt.Errorf("%w: %s: %s", protocol.ErrInvalidParams, path, detail)
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
		Session: sessions.PortableSession{
			ID: art.Session.ID, Title: art.Session.Title, Cwd: art.Session.Cwd, Model: art.Session.Model,
			CreatedAt: art.Session.CreatedAt, UpdatedAt: art.Session.UpdatedAt, Favorite: art.Session.Favorite,
		},
		Messages: messages, Runs: runs, Items: items, ToolResults: toolResults,
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
