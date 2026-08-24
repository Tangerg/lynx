package server

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/toolresult"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/protocol"
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
	selection, err := modelref.New(art.Session.Provider, art.Session.Model)
	if err != nil {
		return sessions.PortableSnapshot{}, invalidArtifact(
			"artifact.session", "provider and model must form a complete selection: %v", err,
		)
	}
	if !selection.Configured() {
		return sessions.PortableSnapshot{}, invalidArtifact(
			"artifact.session", "provider and model are required",
		)
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
	toolResults := make([]toolresult.Blob, 0, len(art.ToolResults))
	for index, encoded := range art.ToolResults {
		id, err := toolresult.ParseID(encoded.ID)
		if err != nil {
			return sessions.PortableSnapshot{}, invalidArtifact(fmt.Sprintf("artifact.toolResults[%d].id", index), "%v", err)
		}
		toolResults = append(toolResults, toolresult.Blob{
			ID: id, SessionID: art.Session.ID, ItemID: encoded.ItemID, ToolName: encoded.ToolName,
			Preview: encoded.Preview, Body: encoded.Body, CreatedAt: encoded.CreatedAt,
		})
	}
	plan, err := portablePlanFromArtifact(art.Plan)
	if err != nil {
		return sessions.PortableSnapshot{}, err
	}
	return sessions.PortableSnapshot{
		Session: sessions.PortableSession{
			ID: art.Session.ID, Title: art.Session.Title, CWD: art.Session.Workspace.Path, Selection: selection,
			CreatedAt: art.Session.CreatedAt, UpdatedAt: art.Session.UpdatedAt, Favorite: art.Session.Favorite,
		},
		Messages: messages, Runs: runs, Items: items, ToolResults: toolResults, Plan: plan,
	}, nil
}

// portablePlanFromArtifact reads the archived Plan. The artifact carries the one
// product value directly, so there is no key union or duplicate-key precedence rule.
func portablePlanFromArtifact(entries []protocol.PlanStep) ([]plan.Step, error) {
	steps := make([]plan.Step, 0, len(entries))
	for index, entry := range entries {
		status, known := planStatusFromWire(entry.Status)
		if !known {
			return nil, invalidArtifact(fmt.Sprintf("artifact.plan[%d].status", index),
				"unknown value %q", entry.Status)
		}
		steps = append(steps, plan.Step{
			Description: entry.Description, Status: status,
		})
	}
	return steps, nil
}

// planStatusFromWire maps the archived status back. It is total over the wire enum
// so an unknown value is refused instead of silently importing as pending — a task
// list whose statuses changed on import is a different plan.
func planStatusFromWire(status protocol.PlanStatus) (plan.Status, bool) {
	switch status {
	case protocol.PlanStatusPending:
		return plan.StatusPending, true
	case protocol.PlanStatusInProgress:
		return plan.StatusInProgress, true
	case protocol.PlanStatusCompleted:
		return plan.StatusCompleted, true
	default:
		return plan.StatusPending, false
	}
}

func portableRunFromArtifact(path string, artifact protocol.ArtifactRun) (sessions.PortableRun, error) {
	outcome, err := portableOutcomeFromArtifact(path+".outcome.type", artifact.Outcome.Type)
	if err != nil {
		return sessions.PortableRun{}, err
	}
	failure, err := portableRunFailureFromArtifact(path+".outcome.error", artifact.Outcome.Error)
	if err != nil {
		return sessions.PortableRun{}, err
	}
	capabilities, err := portableCapabilitiesFromArtifact(path+".protocolProfile", artifact.ProtocolProfile)
	if err != nil {
		return sessions.PortableRun{}, err
	}
	metrics, err := portableMetricsFromArtifact(path+".metrics", artifact.Metrics)
	if err != nil {
		return sessions.PortableRun{}, err
	}
	return sessions.PortableRun{
		SessionID: artifact.SessionID, ID: artifact.ID, SpawnedByItemID: artifact.SpawnedByItemID,
		ParentRunID: artifact.ParentRunID, RootRunID: artifact.RootRunID,
		Provider: artifact.Provider, Model: artifact.Model, Outcome: outcome,
		Failure:       failure,
		Metrics:       metrics,
		ContextTokens: artifact.ContextTokens,
		Limits:        portableLimitsFromArtifact(artifact.Limits),
		Capabilities:  capabilities,
		Detail:        artifact.Outcome.Detail,
		CreatedAt:     artifact.CreatedAt, FinishedAt: artifact.FinishedAt,
		UpdatedAt: artifact.UpdatedAt, MessageMark: artifact.MessageMark,
	}, nil
}

// portableCapabilitiesFromArtifact restores the run's frozen contract, or nothing when
// the archive carried none — which only a child may do, and the aggregate check
// enforces. An interrupt type this runtime cannot raise is refused rather than
// dropped: importing the run without it would silently rewrite the contract the
// archive recorded.
func portableCapabilitiesFromArtifact(path string, profile *protocol.RunProtocolProfile) (*run.Capabilities, error) {
	if profile == nil {
		return nil, nil
	}
	var out run.Capabilities
	for _, feature := range profile.RequiredFeatures {
		switch feature {
		case protocol.RunProtocolFeatureSubagents:
			out.ChildRuns = true
		default:
			return nil, invalidArtifact(path+".requiredFeatures", "unknown value %q", feature)
		}
	}
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

func portableOutcomeFromArtifact(path string, value protocol.ArtifactOutcomeType) (run.Outcome, error) {
	switch value {
	case protocol.ArtifactOutcomeCompleted:
		return run.OutcomeCompleted, nil
	case protocol.ArtifactOutcomeCanceled:
		return run.OutcomeCanceled, nil
	case protocol.ArtifactOutcomeTimedOut:
		return run.OutcomeTimedOut, nil
	case protocol.ArtifactOutcomeFailed:
		return run.OutcomeFailed, nil
	case protocol.ArtifactOutcomeMaxBudget:
		return run.OutcomeMaxBudget, nil
	case protocol.ArtifactOutcomeMaxSteps:
		return run.OutcomeMaxSteps, nil
	case protocol.ArtifactOutcomeLost:
		return run.OutcomeLost, nil
	default:
		return 0, invalidArtifact(path, "unknown value %q", value)
	}
}

func portableMetricsFromArtifact(path string, artifact protocol.ArtifactRunMetrics) (run.Metrics, error) {
	metrics, err := run.NewMetrics(
		portableUsageFromArtifact(artifact.Usage),
		artifact.Steps,
		time.Duration(artifact.ActiveDurationMillis)*time.Millisecond,
	)
	if err != nil {
		return run.Metrics{}, invalidArtifact(path, "%v", err)
	}
	return metrics, nil
}

func portableLimitsFromArtifact(artifact *protocol.ArtifactRunLimits) run.Limits {
	if artifact == nil {
		return run.Limits{}
	}
	return run.Limits{
		MaxTotalTokens: artifact.MaxTotalTokens, MaxSteps: artifact.MaxSteps, MaxBudgetUSD: artifact.MaxBudgetUSD,
	}
}

func portableUsageFromArtifact(artifact *protocol.ArtifactUsage) *accounting.Usage {
	if artifact == nil {
		return nil
	}
	out := &accounting.Usage{Total: accounting.Totals{
		InputTokens: artifact.InputTokens, OutputTokens: artifact.OutputTokens,
		CacheReadTokens: artifact.CacheReadTokens, CacheWriteTokens: artifact.CacheWriteTokens,
		ReasoningTokens: artifact.ReasoningTokens, CostUSD: artifact.CostUSD,
	}}
	if len(artifact.ByModel) != 0 {
		out.ByModel = make(map[string]accounting.Totals, len(artifact.ByModel))
		for model, values := range artifact.ByModel {
			out.ByModel[model] = accounting.Totals{
				InputTokens: values.InputTokens, OutputTokens: values.OutputTokens,
				CacheReadTokens: values.CacheReadTokens, CacheWriteTokens: values.CacheWriteTokens,
				ReasoningTokens: values.ReasoningTokens, CostUSD: values.CostUSD,
			}
		}
	}
	return out
}

func portableItemFromArtifact(sessionID, path string, artifact protocol.ArtifactItem) (transcript.Item, error) {
	if err := artifact.ValidateWire(); err != nil {
		return transcript.Item{}, invalidArtifact(path, "%v", err)
	}
	status, err := portableItemStatus(path+".status", artifact.Status)
	if err != nil {
		return transcript.Item{}, err
	}
	kind, err := portableItemKind(path+".type", artifact.Type)
	if err != nil {
		return transcript.Item{}, err
	}
	failure, err := portableToolFailureFromArtifact(path+".error", artifact.Error)
	if err != nil {
		return transcript.Item{}, err
	}
	snapshot := transcript.ItemSnapshot{
		Identity: transcript.ItemIdentity{
			SessionID: sessionID, ItemID: artifact.ID, RunID: artifact.RunID,
			OccurredAt: artifact.CreatedAt,
		},
		Status: status, Kind: kind, MessagePhase: portableMessagePhase(artifact.Phase),
		Text: artifact.Text, Redacted: artifact.Redacted,
		ApprovalDecision: portableItemApprovalDecision(artifact.ApprovalDecision),
		Failure:          failure, Summary: artifact.Summary, DroppedMessages: artifact.DroppedMessages,
	}
	if kind == transcript.ToolCall {
		snapshot.Identity.OccurredAt = artifact.StartedAt
		if status != transcript.ItemRunning {
			if artifact.DurationMillis != nil {
				if *artifact.DurationMillis > math.MaxInt64/int64(time.Millisecond) {
					return transcript.Item{}, invalidArtifact(path+".durationMillis", "exceeds the representable duration")
				}
				duration := time.Duration(*artifact.DurationMillis) * time.Millisecond
				snapshot.ExecutionDuration = &duration
			}
			snapshot.FinishedAt = artifact.FinishedAt
		}
	}
	safetyClass, err := portableSafetyClass(path+".safetyClass", artifact.SafetyClass)
	if err != nil {
		return transcript.Item{}, err
	}
	snapshot.SafetyClass = safetyClass
	if len(artifact.Content) != 0 {
		snapshot.Content = make([]transcript.ContentBlock, len(artifact.Content))
		for index, block := range artifact.Content {
			content, err := portableContentFromArtifact(fmt.Sprintf("%s.content[%d]", path, index), block)
			if err != nil {
				return transcript.Item{}, err
			}
			snapshot.Content[index] = content
		}
	}
	if artifact.Question != nil {
		question, err := portableQuestionFromArtifact(path+".question", *artifact.Question)
		if err != nil {
			return transcript.Item{}, err
		}
		snapshot.Question = &question
	}
	if artifact.Tool != nil {
		invocation, err := portableToolFromArtifact(path+".tool", *artifact.Tool)
		if err != nil {
			return transcript.Item{}, err
		}
		snapshot.Tool = &invocation
	}
	item, err := transcript.RestoreItem(snapshot)
	if err != nil {
		return transcript.Item{}, invalidArtifact(path, "%v", err)
	}
	return item, nil
}

func portableMessagePhase(phase protocol.MessagePhase) transcript.MessagePhase {
	switch phase {
	case protocol.MessagePhaseCommentary:
		return transcript.MessageCommentary
	case protocol.MessagePhaseFinalAnswer:
		return transcript.MessageFinalAnswer
	default:
		return transcript.MessagePhaseNone
	}
}

func portableItemApprovalDecision(decision protocol.ApprovalDecision) approval.Decision {
	switch decision {
	case protocol.ApprovalApprove:
		return approval.Allow
	case protocol.ApprovalDeny:
		return approval.Deny
	default:
		return ""
	}
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
	decoded, decodeErr := decodeContent(encodedContent{
		kind: artifact.Type, text: artifact.Text, mime: artifact.Mime, data: artifact.Data,
	})
	if decodeErr != nil {
		return transcript.ContentBlock{}, invalidArtifact(path+"."+decodeErr.field, "%s", decodeErr.detail)
	}
	return decoded, nil
}

func portableQuestionFromArtifact(path string, artifact protocol.ArtifactQuestion) (transcript.Question, error) {
	fields := make([]transcript.QuestionField, len(artifact.Fields))
	for index, field := range artifact.Fields {
		kind, err := portableQuestionFieldKind(fmt.Sprintf("%s.fields[%d].type", path, index), field.Type)
		if err != nil {
			return transcript.Question{}, err
		}
		var options []transcript.QuestionOption
		if len(field.Options) > 0 {
			options = make([]transcript.QuestionOption, len(field.Options))
			for optionIndex, option := range field.Options {
				options[optionIndex] = transcript.QuestionOption{Label: option.Label, Description: option.Description, Preview: option.Preview}
			}
		}
		fields[index] = transcript.QuestionField{
			Prompt: field.Prompt, Header: field.Header, Kind: kind,
			Options: options, Multiple: field.Multiple, AllowCustom: field.AllowCustom,
		}
	}
	return transcript.Question{Fields: fields, Answers: cloneAnswers(artifact.Answers)}, nil
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

func portableRunFailureFromArtifact(path string, artifact *protocol.ArtifactProblem) (*run.Failure, error) {
	if artifact == nil {
		return nil, nil
	}
	kind, err := portableRunFailureKind(path+".type", artifact.Type)
	if err != nil {
		return nil, err
	}
	return &run.Failure{
		Kind: kind, Detail: artifact.Detail, DocURL: artifact.DocURL,
		RetryAfter: time.Duration(artifact.RetryAfterSeconds) * time.Second,
	}, nil
}

func portableRunFailureKind(path string, value protocol.ArtifactProblemType) (run.FailureKind, error) {
	switch value {
	case protocol.ArtifactProblemInternalError:
		return run.FailureInternal, nil
	case protocol.ArtifactProblemRunLost:
		return run.FailureLost, nil
	case protocol.ArtifactProblemAgentStuck:
		return run.FailureAgentStuck, nil
	case protocol.ArtifactProblemRateLimited:
		return run.FailureRateLimited, nil
	case protocol.ArtifactProblemInvalidAPIKey:
		return run.FailureInvalidCredentials, nil
	case protocol.ArtifactProblemTimeout:
		return run.FailureTimeout, nil
	case protocol.ArtifactProblemProviderUnavailable:
		return run.FailureProviderUnavailable, nil
	case protocol.ArtifactProblemProviderRejected:
		return run.FailureProviderRejected, nil
	default:
		return 0, invalidArtifact(path, "unknown value %q", value)
	}
}

func portableToolFailureFromArtifact(path string, artifact *protocol.ArtifactProblem) (*tool.Failure, error) {
	if artifact == nil {
		return nil, nil
	}
	var kind tool.FailureKind
	switch artifact.Type {
	case protocol.ArtifactProblemInternalError:
		kind = tool.FailureInternal
	case protocol.ArtifactProblemDeniedByUser:
		kind = tool.FailureDenied
	case protocol.ArtifactProblemToolFailed:
		kind = tool.FailureExecution
	case protocol.ArtifactProblemChildRunCanceled:
		kind = tool.FailureChildRunCanceled
	case protocol.ArtifactProblemToolCanceled:
		kind = tool.FailureCanceled
	default:
		return nil, invalidArtifact(path+".type", "unknown value %q", artifact.Type)
	}
	return &tool.Failure{
		Kind: kind, Detail: artifact.Detail, DocURL: artifact.DocURL,
		RetryAfter: time.Duration(artifact.RetryAfterSeconds) * time.Second,
	}, nil
}
