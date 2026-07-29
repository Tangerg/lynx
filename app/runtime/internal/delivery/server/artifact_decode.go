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
	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
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
	todos, err := portableTodosFromArtifact(art.States)
	if err != nil {
		return sessions.PortableSnapshot{}, err
	}
	return sessions.PortableSnapshot{
		Session: sessions.PortableSession{
			ID: art.Session.ID, Title: art.Session.Title, Cwd: art.Session.Cwd, Model: art.Session.Model,
			CreatedAt: art.Session.CreatedAt, UpdatedAt: art.Session.UpdatedAt, Favorite: art.Session.Favorite,
		},
		Messages: messages, Runs: runs, Items: items, ToolResults: toolResults, Todos: todos,
	}, nil
}

// portableTodosFromArtifact reads the archived task list. The states array is a MAP
// of keys to values, so a repeated key is refused rather than resolved by order:
// two answers to "what was the task list" is not a list the import may pick from.
func portableTodosFromArtifact(states []protocol.ArtifactState) ([]todo.Item, error) {
	var todos []todo.Item
	seen := make(map[protocol.ArtifactStateType]bool, len(states))
	for index, state := range states {
		path := fmt.Sprintf("artifact.states[%d]", index)
		if seen[state.Type] {
			return nil, invalidArtifact(path+".type", "repeats state key %q", state.Type)
		}
		seen[state.Type] = true
		switch state.Type {
		case protocol.ArtifactStateTodos:
			for itemIndex, entry := range state.Todos {
				status, known := todoStatusFromWire(entry.Status)
				if !known {
					return nil, invalidArtifact(fmt.Sprintf("%s.todos[%d].status", path, itemIndex),
						"unknown value %q", entry.Status)
				}
				todos = append(todos, todo.Item{
					Content: entry.Text, Status: status,
					BlockedReason: entry.BlockedReason, NextAction: entry.NextAction,
				})
			}
		default:
			return nil, invalidArtifact(path+".type", "unknown value %q", state.Type)
		}
	}
	return todos, nil
}

// todoStatusFromWire maps the archived status back. It is total over the wire enum
// so an unknown value is refused instead of silently importing as pending — a task
// list whose statuses changed on import is a different plan.
func todoStatusFromWire(status protocol.TodoStatus) (todo.Status, bool) {
	switch status {
	case protocol.TodoStatusPending:
		return todo.StatusPending, true
	case protocol.TodoStatusInProgress:
		return todo.StatusInProgress, true
	case protocol.TodoStatusCompleted:
		return todo.StatusCompleted, true
	default:
		return todo.StatusPending, false
	}
}

func portableRunFromArtifact(path string, artifact protocol.ArtifactRun) (sessions.PortableRun, error) {
	outcome, err := portableOutcomeFromArtifact(path+".outcome.type", artifact.Outcome.Type)
	if err != nil {
		return sessions.PortableRun{}, err
	}
	problem, err := portableProblemFromArtifact(path+".outcome.error", artifact.Outcome.Error, transcript.RunProblem)
	if err != nil {
		return sessions.PortableRun{}, err
	}
	profile, err := portableProfileFromArtifact(path+".protocolProfile", artifact.ProtocolProfile)
	if err != nil {
		return sessions.PortableRun{}, err
	}
	return sessions.PortableRun{
		SessionID: artifact.SessionID, ID: artifact.ID, SpawnedByItemID: artifact.SpawnedByItemID,
		ParentRunID: artifact.ParentRunID, RootRunID: artifact.RootRunID,
		Provider: artifact.Provider, Model: artifact.Model, Outcome: outcome,
		Error:           problem,
		Metrics:         portableMetricsFromArtifact(artifact.Metrics),
		Limits:          portableLimitsFromArtifact(artifact.Limits),
		ProtocolProfile: profile,
		Detail:          artifact.Outcome.Detail,
		CreatedAt:       artifact.CreatedAt, FinishedAt: artifact.FinishedAt,
		UpdatedAt: artifact.UpdatedAt, MessageMark: artifact.MessageMark,
	}, nil
}

// portableProfileFromArtifact restores the run's frozen contract, or nothing when
// the archive carried none — which only a child may do, and the aggregate check
// enforces. An interrupt type this runtime cannot raise is refused rather than
// dropped: importing the run without it would silently rewrite the contract the
// archive recorded.
func portableProfileFromArtifact(path string, profile *protocol.RunProtocolProfile) (*execution.RunProtocolProfile, error) {
	if profile == nil {
		return nil, nil
	}
	out := execution.RunProtocolProfile{RequiredFeatures: profile.RequiredFeatures}
	for _, declared := range profile.InterruptTypes {
		kind, backed := interruptKindFromWire(declared)
		if !backed {
			return nil, invalidArtifact(path+".interruptTypes", "unknown value %q", declared)
		}
		out.InterruptKinds = append(out.InterruptKinds, kind)
	}
	normalized := out.Normalized()
	return &normalized, nil
}

func portableOutcomeFromArtifact(path string, value protocol.ArtifactOutcomeType) (execution.Outcome, error) {
	switch value {
	case protocol.ArtifactOutcomeCompleted:
		return execution.OutcomeCompleted, nil
	case protocol.ArtifactOutcomeCanceled:
		return execution.OutcomeCanceled, nil
	case protocol.ArtifactOutcomeError:
		return execution.OutcomeError, nil
	case protocol.ArtifactOutcomeMaxBudget:
		return execution.OutcomeMaxBudget, nil
	case protocol.ArtifactOutcomeMaxSteps:
		return execution.OutcomeMaxSteps, nil
	default:
		return 0, invalidArtifact(path, "unknown value %q", value)
	}
}

func portableMetricsFromArtifact(artifact protocol.ArtifactRunMetrics) transcript.RunMetrics {
	return transcript.RunMetrics{
		Usage:          portableUsageFromArtifact(artifact.Usage),
		Steps:          artifact.Steps,
		ActiveDuration: time.Duration(artifact.ActiveDurationMs) * time.Millisecond,
	}
}

func portableLimitsFromArtifact(artifact *protocol.ArtifactRunLimits) execution.RunLimits {
	if artifact == nil {
		return execution.RunLimits{}
	}
	return execution.RunLimits{MaxSteps: artifact.MaxSteps, MaxBudgetUSD: artifact.MaxBudgetUSD}
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
		Error:   problem,
		Summary: artifact.Summary, DroppedMessages: artifact.DroppedMessages,
	}
	safetyClass, err := portableSafetyClass(path+".safetyClass", artifact.SafetyClass)
	if err != nil {
		return transcript.Item{}, err
	}
	out.SafetyClass = safetyClass
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
			stepStatus, err := portablePlanStepStatus(fmt.Sprintf("%s.steps[%d].status", path, index), step.Status)
			if err != nil {
				return transcript.Item{}, err
			}
			out.Steps[index] = transcript.PlanStep{ID: step.ID, Title: step.Title, Status: stepStatus}
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

func portableItemStatus(path string, value protocol.ItemStatus) (transcript.ItemStatus, error) {
	switch value {
	case protocol.ItemStatusRunning:
		return transcript.ItemRunning, nil
	case protocol.ItemStatusCompleted:
		return transcript.ItemCompleted, nil
	case protocol.ItemStatusIncomplete:
		return transcript.ItemIncomplete, nil
	default:
		return 0, invalidArtifact(path, "unknown value %q", value)
	}
}

func portableItemKind(path string, value protocol.ItemType) (transcript.ItemKind, error) {
	switch value {
	case protocol.ItemTypeUserMessage:
		return transcript.UserMessage, nil
	case protocol.ItemTypeAgentMessage:
		return transcript.AgentMessage, nil
	case protocol.ItemTypeReasoning:
		return transcript.Reasoning, nil
	case protocol.ItemTypePlan:
		return transcript.Plan, nil
	case protocol.ItemTypeQuestion:
		return transcript.QuestionItem, nil
	case protocol.ItemTypeToolCall:
		return transcript.ToolCall, nil
	case protocol.ItemTypeCompaction:
		return transcript.Compaction, nil
	default:
		return 0, invalidArtifact(path, "unknown value %q", value)
	}
}

func portableContentFromArtifact(path string, artifact protocol.ArtifactContentBlock) (transcript.ContentBlock, error) {
	switch artifact.Type {
	case protocol.ContentBlockText:
		return transcript.ContentBlock{Kind: transcript.TextContent, Text: artifact.Text, Mime: artifact.Mime, Data: artifact.Data}, nil
	case protocol.ContentBlockImage:
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

func portableQuestionFieldKind(path string, value protocol.QuestionFieldType) (transcript.QuestionFieldKind, error) {
	switch value {
	case protocol.QuestionFieldText:
		return transcript.QuestionText, nil
	case protocol.QuestionFieldChoice:
		return transcript.QuestionChoice, nil
	default:
		return 0, invalidArtifact(path, "unknown value %q", value)
	}
}

func portableSafetyClass(path string, value protocol.SafetyClass) (tool.SafetyClass, error) {
	switch value {
	case "":
		return "", nil
	case protocol.SafetyClassSafe:
		return tool.SafetyClassSafe, nil
	case protocol.SafetyClassWrite:
		return tool.SafetyClassWrite, nil
	case protocol.SafetyClassExec:
		return tool.SafetyClassExec, nil
	case protocol.SafetyClassNetwork:
		return tool.SafetyClassNetwork, nil
	default:
		return "", invalidArtifact(path, "unknown value %q", value)
	}
}

func portablePlanStepStatus(path string, value protocol.PlanStepStatus) (transcript.PlanStepStatus, error) {
	switch value {
	case protocol.PlanStepPending:
		return transcript.PlanStepPending, nil
	case protocol.PlanStepRunning:
		return transcript.PlanStepRunning, nil
	case protocol.PlanStepCompleted:
		return transcript.PlanStepCompleted, nil
	case protocol.PlanStepFailed:
		return transcript.PlanStepFailed, nil
	default:
		return "", invalidArtifact(path, "unknown value %q", value)
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
		RetryAfterSeconds: artifact.RetryAfterSeconds,
	}, nil
}

func portableProblemKind(path string, value protocol.ArtifactProblemType) (transcript.ProblemKind, error) {
	switch value {
	case protocol.ArtifactProblemInternalError:
		return transcript.InternalProblem, nil
	case protocol.ArtifactProblemRunLost:
		return transcript.RunLostProblem, nil
	case protocol.ArtifactProblemAgentStuck:
		return transcript.AgentStuckProblem, nil
	case protocol.ArtifactProblemRateLimited:
		return transcript.RateLimitedProblem, nil
	case protocol.ArtifactProblemInvalidAPIKey:
		return transcript.InvalidAPIKeyProblem, nil
	case protocol.ArtifactProblemTimeout:
		return transcript.TimeoutProblem, nil
	case protocol.ArtifactProblemProviderUnavailable:
		return transcript.ProviderUnavailableProblem, nil
	case protocol.ArtifactProblemProviderRejected:
		return transcript.ProviderRejectedProblem, nil
	case protocol.ArtifactProblemDeniedByUser:
		return transcript.DeniedByUserProblem, nil
	case protocol.ArtifactProblemToolFailed:
		return transcript.ToolFailedProblem, nil
	default:
		return 0, invalidArtifact(path, "unknown value %q", value)
	}
}
