package server

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/toolresult"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
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
	plan, err := portablePlanFromArtifact(art.States)
	if err != nil {
		return sessions.PortableSnapshot{}, err
	}
	return sessions.PortableSnapshot{
		Session: sessions.PortableSession{
			ID: art.Session.ID, Title: art.Session.Title, CWD: art.Session.Workspace.Path, Model: art.Session.Model,
			CreatedAt: art.Session.CreatedAt, UpdatedAt: art.Session.UpdatedAt, Favorite: art.Session.Favorite,
		},
		Messages: messages, Runs: runs, Items: items, ToolResults: toolResults, Plan: plan,
	}, nil
}

// portablePlanFromArtifact reads the archived Plan. The states array is a MAP
// of keys to values, so a repeated key is refused rather than resolved by order:
// two answers to "what was the Plan" is not a list the import may pick from.
func portablePlanFromArtifact(states []protocol.ArtifactState) ([]plan.Step, error) {
	var steps []plan.Step
	seen := make(map[protocol.ArtifactStateType]bool, len(states))
	for index, state := range states {
		path := fmt.Sprintf("artifact.states[%d]", index)
		if seen[state.Type] {
			return nil, invalidArtifact(path+".type", "repeats state key %q", state.Type)
		}
		seen[state.Type] = true
		switch state.Type {
		case protocol.ArtifactStatePlan:
			for stepIndex, entry := range state.Plan {
				status, known := planStatusFromWire(entry.Status)
				if !known {
					return nil, invalidArtifact(fmt.Sprintf("%s.plan[%d].status", path, stepIndex),
						"unknown value %q", entry.Status)
				}
				steps = append(steps, plan.Step{
					Description: entry.Description, Status: status,
				})
			}
		default:
			return nil, invalidArtifact(path+".type", "unknown value %q", state.Type)
		}
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
	problem, err := portableProblemFromArtifact(path+".outcome.error", artifact.Outcome.Error, transcript.RunProblem)
	if err != nil {
		return sessions.PortableRun{}, err
	}
	capabilities, err := portableCapabilitiesFromArtifact(path+".protocolProfile", artifact.ProtocolProfile)
	if err != nil {
		return sessions.PortableRun{}, err
	}
	return sessions.PortableRun{
		SessionID: artifact.SessionID, ID: artifact.ID, SpawnedByItemID: artifact.SpawnedByItemID,
		ParentRunID: artifact.ParentRunID, RootRunID: artifact.RootRunID,
		Provider: artifact.Provider, Model: artifact.Model, Outcome: outcome,
		Error:        problem,
		Metrics:      portableMetricsFromArtifact(artifact.Metrics),
		Limits:       portableLimitsFromArtifact(artifact.Limits),
		Capabilities: capabilities,
		Detail:       artifact.Outcome.Detail,
		CreatedAt:    artifact.CreatedAt, FinishedAt: artifact.FinishedAt,
		UpdatedAt: artifact.UpdatedAt, MessageMark: artifact.MessageMark,
	}, nil
}

// portableCapabilitiesFromArtifact restores the run's frozen contract, or nothing when
// the archive carried none — which only a child may do, and the aggregate check
// enforces. An interrupt type this runtime cannot raise is refused rather than
// dropped: importing the run without it would silently rewrite the contract the
// archive recorded.
func portableCapabilitiesFromArtifact(path string, profile *protocol.RunProtocolProfile) (*run.RunCapabilities, error) {
	if profile == nil {
		return nil, nil
	}
	var out run.RunCapabilities
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

func portableMetricsFromArtifact(artifact protocol.ArtifactRunMetrics) transcript.RunMetrics {
	return transcript.RunMetrics{
		Usage:          portableUsageFromArtifact(artifact.Usage),
		Steps:          artifact.Steps,
		ActiveDuration: time.Duration(artifact.ActiveDurationMillis) * time.Millisecond,
	}
}

func portableLimitsFromArtifact(artifact *protocol.ArtifactRunLimits) run.RunLimits {
	if artifact == nil {
		return run.RunLimits{}
	}
	return run.RunLimits{
		MaxTotalTokens: artifact.MaxTotalTokens, MaxSteps: artifact.MaxSteps, MaxBudgetUSD: artifact.MaxBudgetUSD,
	}
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
	problem, err := portableProblemFromArtifact(path+".error", artifact.Error, transcript.ToolProblem)
	if err != nil {
		return transcript.Item{}, err
	}
	out := transcript.Item{
		SessionID: sessionID, ID: artifact.ID, RunID: artifact.RunID, Status: status, Kind: kind,
		OccurredAt: artifact.CreatedAt, Text: artifact.Text, Redacted: artifact.Redacted,
		Error:   problem,
		Summary: artifact.Summary, DroppedMessages: artifact.DroppedMessages,
	}
	if kind == transcript.ToolCall {
		out.OccurredAt = artifact.StartedAt
		if status != transcript.ItemRunning {
			expectedDuration := artifact.FinishedAt.Sub(artifact.StartedAt).Milliseconds()
			if artifact.DurationMillis == nil || *artifact.DurationMillis != expectedDuration {
				return transcript.Item{}, invalidArtifact(path+".durationMillis", "must equal finishedAt minus startedAt in milliseconds")
			}
			out.FinishedAt = artifact.FinishedAt
		}
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
	return transcript.Question{Fields: fields}, nil
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
	case protocol.ArtifactProblemChildRunCanceled:
		return transcript.ChildRunCanceledProblem, nil
	default:
		return 0, invalidArtifact(path, "unknown value %q", value)
	}
}
