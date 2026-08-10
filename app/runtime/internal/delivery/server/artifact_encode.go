package server

import (
	"encoding/json"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// artifactFromPortable maps the terminal archive projection to the versioned
// protocol document. Tool results remain the canonical values stored in the
// transcript; archive encoding does not reinterpret them.
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
		States: artifactStatesFromPortable(portable),
	}, nil
}

// artifactStatesFromPortable carries the session-scoped projections that have a
// value. An empty list is omitted rather than written as an entry with nothing in
// it: "no Plan" and "an empty Plan" are the same fact here, and only the
// live projection's revision distinguishes them — which an archive deliberately
// does not carry.
func artifactStatesFromPortable(portable sessions.PortableSnapshot) []protocol.ArtifactState {
	if len(portable.Plan) == 0 {
		return nil
	}
	return []protocol.ArtifactState{{
		Type: protocol.ArtifactStatePlan,
		Plan: presentPlanSnapshots(portable.Plan),
	}}
}

func artifactSessionFromPortable(value sessions.PortableSession) protocol.ArtifactSession {
	return protocol.ArtifactSession{
		ID: value.ID, Title: value.Title, Workspace: protocol.WorkspaceRef{Path: value.CWD}, Model: value.Model,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt, Favorite: value.Favorite,
	}
}

func artifactRunFromPortable(run sessions.PortableRun) (protocol.ArtifactRun, error) {
	outcome, err := artifactOutcomeType(run.Outcome)
	if err != nil {
		return protocol.ArtifactRun{}, fmt.Errorf("run %q outcome: %w", run.ID, err)
	}
	problem, err := artifactRunFailureFromDomain(run.Failure)
	if err != nil {
		return protocol.ArtifactRun{}, fmt.Errorf("run %q failure: %w", run.ID, err)
	}
	return protocol.ArtifactRun{
		ID: run.ID, SessionID: run.SessionID, SpawnedByItemID: run.SpawnedByItemID,
		Provider: run.Provider, Model: run.Model,
		ParentRunID:     run.ParentRunID,
		RootRunID:       run.RootRunID,
		Limits:          artifactLimitsFromDomain(run.Limits),
		Metrics:         artifactMetricsFromDomain(run.Metrics),
		ProtocolProfile: presentArtifactProtocolProfile(run.Capabilities),
		Outcome: protocol.ArtifactOutcome{
			Type: outcome, Error: problem, Detail: run.Detail,
		},
		CreatedAt: run.CreatedAt, FinishedAt: run.FinishedAt,
		UpdatedAt: run.UpdatedAt, MessageMark: run.MessageMark,
	}, nil
}

// presentArtifactProtocolProfile writes the protocol contract a root Run
// published under, and
// nothing for a child — a child reads its root's, and writing a second copy is how
// the two come to disagree.
func presentArtifactProtocolProfile(capabilities *run.Capabilities) *protocol.RunProtocolProfile {
	if capabilities == nil {
		return nil
	}
	presented := presentRunProtocolProfile(*capabilities)
	return &presented
}

func artifactOutcomeType(outcome run.Outcome) (protocol.ArtifactOutcomeType, error) {
	switch outcome {
	case run.OutcomeCompleted:
		return protocol.ArtifactOutcomeCompleted, nil
	case run.OutcomeCanceled:
		return protocol.ArtifactOutcomeCanceled, nil
	case run.OutcomeTimedOut:
		return protocol.ArtifactOutcomeTimedOut, nil
	case run.OutcomeFailed:
		return protocol.ArtifactOutcomeFailed, nil
	case run.OutcomeMaxBudget:
		return protocol.ArtifactOutcomeMaxBudget, nil
	case run.OutcomeMaxSteps:
		return protocol.ArtifactOutcomeMaxSteps, nil
	case run.OutcomeLost:
		return protocol.ArtifactOutcomeLost, nil
	default:
		return "", fmt.Errorf("unknown value %d", outcome)
	}
}

func artifactMetricsFromDomain(metrics run.Metrics) protocol.ArtifactRunMetrics {
	usage, reported := metrics.Usage()
	var usageRef *accounting.Usage
	if reported {
		usageRef = &usage
	}
	return protocol.ArtifactRunMetrics{
		Usage:                artifactUsageFromDomain(usageRef),
		Steps:                metrics.Steps(),
		ActiveDurationMillis: metrics.ActiveDuration().Milliseconds(),
	}
}

func artifactLimitsFromDomain(limits run.Limits) *protocol.ArtifactRunLimits {
	if limits.IsZero() {
		return nil
	}
	return &protocol.ArtifactRunLimits{
		MaxTotalTokens: limits.MaxTotalTokens, MaxSteps: limits.MaxSteps, MaxBudgetUSD: limits.MaxBudgetUSD,
	}
}

func artifactUsageFromDomain(usage *accounting.Usage) *protocol.ArtifactUsage {
	if usage == nil {
		return nil
	}
	out := &protocol.ArtifactUsage{
		InputTokens: usage.Total.InputTokens, OutputTokens: usage.Total.OutputTokens,
		CacheReadTokens: usage.Total.CacheReadTokens, CacheWriteTokens: usage.Total.CacheWriteTokens,
		ReasoningTokens: usage.Total.ReasoningTokens, CostUSD: usage.Total.CostUSD,
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

func artifactRunFailureFromDomain(failure *run.Failure) (*protocol.ArtifactProblem, error) {
	if failure == nil {
		return nil, nil
	}
	kind, err := artifactRunFailureType(failure.Kind)
	if err != nil {
		return nil, err
	}
	return &protocol.ArtifactProblem{
		Type: kind, Detail: failure.Detail, DocURL: failure.DocURL,
		RetryAfterSeconds: int(failure.RetryAfter.Seconds()),
	}, nil
}

func artifactRunFailureType(kind run.FailureKind) (protocol.ArtifactProblemType, error) {
	switch kind {
	case run.FailureInternal:
		return protocol.ArtifactProblemInternalError, nil
	case run.FailureLost:
		return protocol.ArtifactProblemRunLost, nil
	case run.FailureAgentStuck:
		return protocol.ArtifactProblemAgentStuck, nil
	case run.FailureRateLimited:
		return protocol.ArtifactProblemRateLimited, nil
	case run.FailureInvalidCredentials:
		return protocol.ArtifactProblemInvalidAPIKey, nil
	case run.FailureTimeout:
		return protocol.ArtifactProblemTimeout, nil
	case run.FailureProviderUnavailable:
		return protocol.ArtifactProblemProviderUnavailable, nil
	case run.FailureProviderRejected:
		return protocol.ArtifactProblemProviderRejected, nil
	default:
		return "", fmt.Errorf("unknown value %d", kind)
	}
}

func artifactToolFailureFromDomain(failure *tool.Failure) (*protocol.ArtifactProblem, error) {
	if failure == nil {
		return nil, nil
	}
	var kind protocol.ArtifactProblemType
	switch failure.Kind {
	case tool.FailureInternal:
		kind = protocol.ArtifactProblemInternalError
	case tool.FailureDenied:
		kind = protocol.ArtifactProblemDeniedByUser
	case tool.FailureExecution:
		kind = protocol.ArtifactProblemToolFailed
	case tool.FailureChildRunCanceled:
		kind = protocol.ArtifactProblemChildRunCanceled
	default:
		return nil, fmt.Errorf("unknown value %d", failure.Kind)
	}
	return &protocol.ArtifactProblem{
		Type: kind, Detail: failure.Detail, DocURL: failure.DocURL,
		RetryAfterSeconds: int(failure.RetryAfter.Seconds()),
	}, nil
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
	problem, err := artifactToolFailureFromDomain(item.Error)
	if err != nil {
		return protocol.ArtifactItem{}, fmt.Errorf("item %q error: %w", item.ID, err)
	}
	safetyClass, err := artifactSafetyClass(item.SafetyClass)
	if err != nil {
		return protocol.ArtifactItem{}, fmt.Errorf("item %q safety class: %w", item.ID, err)
	}
	out := protocol.ArtifactItem{
		ID: item.ID, RunID: item.RunID, Status: status,
		Type: kind, Text: item.Text, Redacted: item.Redacted,
		SafetyClass: safetyClass, Error: problem,
		Summary: item.Summary, DroppedMessages: item.DroppedMessages,
	}
	if len(item.Content) != 0 {
		out.Content = make([]protocol.ArtifactContentBlock, len(item.Content))
		for index, block := range item.Content {
			encoded, err := encodeContent(block)
			if err != nil {
				return protocol.ArtifactItem{}, fmt.Errorf("item %q content %d: %w", item.ID, index, err)
			}
			out.Content[index] = protocol.ArtifactContentBlock{Type: encoded.kind, Text: encoded.text, Mime: encoded.mime, Data: encoded.data}
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
	if item.Kind == transcript.ToolCall {
		out.StartedAt = item.OccurredAt
		out.FinishedAt = item.FinishedAt
		out.DurationMillis = presentToolDurationMillis(item)
	} else {
		out.CreatedAt = item.OccurredAt
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

func artifactQuestionFromDomain(question transcript.Question) (*protocol.ArtifactQuestion, error) {
	fields := make([]protocol.ArtifactQuestionField, len(question.Fields))
	for index, field := range question.Fields {
		var options []protocol.ArtifactQuestionOption
		if len(field.Options) > 0 {
			options = make([]protocol.ArtifactQuestionOption, len(field.Options))
			for optionIndex, option := range field.Options {
				options[optionIndex] = protocol.ArtifactQuestionOption{Label: option.Label, Description: option.Description, Preview: option.Preview}
			}
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
			Prompt: field.Prompt, Header: field.Header, Type: fieldType,
			Options: options, Multiple: field.Multiple, AllowCustom: field.AllowCustom,
		}
	}
	return &protocol.ArtifactQuestion{Fields: fields}, nil
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
