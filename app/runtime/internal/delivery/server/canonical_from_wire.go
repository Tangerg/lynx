package server

import (
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

func canonicalArtifact(art protocol.SessionArtifact, messageCount int) ([]transcript.Run, []transcript.Item, error) {
	runs := make([]transcript.Run, 0, len(art.Runs))
	runIDs := make(map[string]struct{}, len(art.Runs))
	runsByID := make(map[string]transcript.Run, len(art.Runs))
	for i, entry := range art.Runs {
		path := fmt.Sprintf("artifact.runs[%d]", i)
		if _, duplicate := runIDs[entry.Run.ID]; entry.Run.ID != "" && duplicate {
			return nil, nil, invalidArtifact(path+".run.id", "duplicate id %q", entry.Run.ID)
		}
		if entry.MessageMark < 0 || entry.MessageMark > messageCount {
			return nil, nil, invalidArtifact(path+".messageMark", "must be between 0 and %d", messageCount)
		}
		run, err := canonicalRunFromWire(art.Session.ID, path+".run", entry.Run, entry.UpdatedAt, entry.MessageMark)
		if err != nil {
			return nil, nil, err
		}
		runIDs[run.ID] = struct{}{}
		runsByID[run.ID] = run
		runs = append(runs, run)
	}

	items := make([]transcript.Item, 0, len(art.Items))
	itemIDs := make(map[string]transcript.Item, len(art.Items))
	for i, entry := range art.Items {
		path := fmt.Sprintf("artifact.items[%d].item", i)
		if _, duplicate := itemIDs[entry.Item.ID]; entry.Item.ID != "" && duplicate {
			return nil, nil, invalidArtifact(path+".id", "duplicate id %q", entry.Item.ID)
		}
		item, err := canonicalItemFromWire(art.Session.ID, path, entry.Item)
		if err != nil {
			return nil, nil, err
		}
		if _, ok := runIDs[item.RunID]; !ok {
			return nil, nil, invalidArtifact(path+".runId", "references unknown run %q", item.RunID)
		}
		itemIDs[item.ID] = item
		items = append(items, item)
	}

	for i, run := range runs {
		path := fmt.Sprintf("artifact.runs[%d].run", i)
		if run.SpawnedByItemID != "" {
			item, ok := itemIDs[run.SpawnedByItemID]
			if !ok {
				return nil, nil, invalidArtifact(path+".spawnedByItemId", "references unknown item %q", run.SpawnedByItemID)
			}
			if item.Kind != transcript.ToolCall {
				return nil, nil, invalidArtifact(path+".spawnedByItemId", "must reference a toolCall item")
			}
			if item.RunID == run.ID {
				return nil, nil, invalidArtifact(path+".spawnedByItemId", "cannot reference an item from the spawned run itself")
			}
		}
	}
	if err := validateRunTree(runs, itemIDs); err != nil {
		return nil, nil, err
	}
	for i, item := range items {
		if runsByID[item.RunID].State.IsTerminal() && item.Status == transcript.ItemRunning {
			return nil, nil, invalidArtifact(
				fmt.Sprintf("artifact.items[%d].item.status", i),
				"cannot be running after its run has terminated",
			)
		}
	}
	return runs, items, nil
}

func validateRunTree(runs []transcript.Run, items map[string]transcript.Item) error {
	parents := make(map[string]string, len(runs))
	paths := make(map[string]string, len(runs))
	for i, run := range runs {
		if run.SpawnedByItemID == "" {
			continue
		}
		parents[run.ID] = items[run.SpawnedByItemID].RunID
		paths[run.ID] = fmt.Sprintf("artifact.runs[%d].run.spawnedByItemId", i)
	}
	states := make(map[string]uint8, len(parents))
	for origin := range parents {
		if states[origin] == 2 {
			continue
		}
		var path []string
		for current := origin; current != ""; current = parents[current] {
			if states[current] == 1 {
				return invalidArtifact(paths[origin], "creates a cycle in the run tree")
			}
			if states[current] == 2 {
				break
			}
			states[current] = 1
			path = append(path, current)
		}
		for _, runID := range path {
			states[runID] = 2
		}
	}
	return nil
}

func canonicalRunFromWire(sessionID, path string, ref protocol.RunRef, updatedAt time.Time, messageMark int) (transcript.Run, error) {
	if ref.ID == "" {
		return transcript.Run{}, invalidArtifact(path+".id", "is required")
	}
	if ref.SessionID != sessionID {
		return transcript.Run{}, invalidArtifact(path+".sessionId", "must equal artifact.session.id")
	}
	run := transcript.Run{
		SessionID: sessionID, ID: ref.ID, SpawnedByItemID: ref.SpawnedByItemID,
		Provider: ref.Provider, Model: ref.Model, State: execution.Running,
		CreatedAt: ref.CreatedAt, FinishedAt: ref.FinishedAt,
		UpdatedAt: updatedAt, MessageMark: messageMark,
	}
	switch ref.Status {
	case protocol.RunStatusRunning:
		return transcript.Run{}, invalidArtifact(path+".status", "running runs are not portable")
	case protocol.RunStatusFinished:
		if ref.Outcome == nil {
			return transcript.Run{}, invalidArtifact(path+".outcome", "is required while status is finished")
		}
		if ref.FinishedAt.IsZero() {
			return transcript.Run{}, invalidArtifact(path+".finishedAt", "is required while status is finished")
		}
	default:
		return transcript.Run{}, invalidArtifact(path+".status", "unknown value %q", ref.Status)
	}

	if ref.Outcome.Type == protocol.OutcomeInterrupt {
		return transcript.Run{}, invalidArtifact(path+".outcome.type", "interrupted runs are not portable")
	}
	if len(ref.Outcome.Interrupts) != 0 {
		return transcript.Run{}, invalidArtifact(path+".outcome.interrupts", "must be absent for terminal outcome %q", ref.Outcome.Type)
	}
	outcome, err := canonicalOutcome(path+".outcome.type", ref.Outcome.Type)
	if err != nil {
		return transcript.Run{}, err
	}
	state, ok := execution.Running.Terminate(outcome)
	if !ok {
		return transcript.Run{}, invalidArtifact(path+".outcome.type", "cannot terminate a running run")
	}
	result, err := canonicalRunResult(path+".outcome.result", ref.Outcome.Result, outcome)
	if err != nil {
		return transcript.Run{}, err
	}
	run.State = state
	run.Outcome = new(outcome)
	run.Result = result
	run.Detail = ref.Outcome.Detail
	return run, nil
}

func canonicalOutcome(path string, kind protocol.RunOutcomeType) (execution.Outcome, error) {
	switch kind {
	case protocol.OutcomeCompleted:
		return execution.OutcomeCompleted, nil
	case protocol.OutcomeCanceled:
		return execution.OutcomeCanceled, nil
	case protocol.OutcomeError:
		return execution.OutcomeError, nil
	case protocol.OutcomeMaxBudget:
		return execution.OutcomeMaxBudget, nil
	case protocol.OutcomeMaxSteps:
		return execution.OutcomeMaxSteps, nil
	default:
		return 0, invalidArtifact(path, "unknown value %q", kind)
	}
}

func canonicalRunResult(path string, result *protocol.RunResult, outcome execution.Outcome) (*transcript.RunResult, error) {
	if result == nil {
		return nil, invalidArtifact(path, "is required for terminal outcome %q", outcome.String())
	}
	if result.DurationMs < 0 {
		return nil, invalidArtifact(path+".durationMs", "must not be negative")
	}
	steps := 0
	if result.Steps != nil {
		if *result.Steps < 0 {
			return nil, invalidArtifact(path+".steps", "must not be negative")
		}
		steps = *result.Steps
	}
	usage, err := canonicalUsage(path+".usage", result.Usage)
	if err != nil {
		return nil, err
	}
	problem, err := canonicalProblem(path+".error", result.Error, protocol.ErrorChannelRun)
	if err != nil {
		return nil, err
	}
	if outcome == execution.OutcomeError && problem == nil {
		return nil, invalidArtifact(path+".error", "is required for outcome error")
	}
	if outcome != execution.OutcomeError && problem != nil {
		return nil, invalidArtifact(path+".error", "must be absent for outcome %q", outcome.String())
	}
	return &transcript.RunResult{
		Usage: usage, Steps: steps, Error: problem,
		Duration: time.Duration(result.DurationMs) * time.Millisecond,
	}, nil
}

func canonicalUsage(path string, usage *protocol.Usage) (*transcript.Usage, error) {
	if usage == nil {
		return nil, nil
	}
	total, err := canonicalModelUsage(path, usage.ModelUsage)
	if err != nil {
		return nil, err
	}
	out := &transcript.Usage{ModelUsage: total}
	if len(usage.ByModel) > 0 {
		out.ByModel = make(map[string]transcript.ModelUsage, len(usage.ByModel))
		for model, modelUsage := range usage.ByModel {
			if model == "" {
				return nil, invalidArtifact(path+".byModel", "contains an empty model id")
			}
			converted, err := canonicalModelUsage(path+".byModel["+model+"]", modelUsage)
			if err != nil {
				return nil, err
			}
			out.ByModel[model] = converted
		}
	}
	return out, nil
}

func canonicalModelUsage(path string, usage protocol.ModelUsage) (transcript.ModelUsage, error) {
	if usage.InputTokens < 0 || usage.OutputTokens < 0 || usage.CacheReadTokens < 0 || usage.CacheWriteTokens < 0 || usage.ReasoningTokens < 0 {
		return transcript.ModelUsage{}, invalidArtifact(path, "token counts must not be negative")
	}
	if usage.CostUSD != nil && *usage.CostUSD < 0 {
		return transcript.ModelUsage{}, invalidArtifact(path+".costUsd", "must not be negative")
	}
	return transcript.ModelUsage{
		InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens,
		CacheReadTokens: usage.CacheReadTokens, CacheWriteTokens: usage.CacheWriteTokens,
		ReasoningTokens: usage.ReasoningTokens, CostUSD: usage.CostUSD,
	}, nil
}

func canonicalItemFromWire(sessionID, path string, item protocol.Item) (transcript.Item, error) {
	if item.ID == "" {
		return transcript.Item{}, invalidArtifact(path+".id", "is required")
	}
	if item.RunID == "" {
		return transcript.Item{}, invalidArtifact(path+".runId", "is required")
	}
	status, err := canonicalItemStatus(path+".status", item.Status)
	if err != nil {
		return transcript.Item{}, err
	}
	kind, err := canonicalItemKind(path+".type", item.Type)
	if err != nil {
		return transcript.Item{}, err
	}
	if err := validateSafetyClass(path+".safetyClass", item.SafetyClass); err != nil {
		return transcript.Item{}, err
	}
	problem, err := canonicalProblem(path+".error", item.Error, protocol.ErrorChannelTool)
	if err != nil {
		return transcript.Item{}, err
	}
	if problem != nil && kind != transcript.ToolCall {
		return transcript.Item{}, invalidArtifact(path+".error", "is only valid for a toolCall item")
	}
	out := transcript.Item{
		SessionID: sessionID, ID: item.ID, RunID: item.RunID,
		Status: status, Kind: kind, CreatedAt: item.CreatedAt,
		Text: item.Text, Redacted: item.Redacted,
		SafetyClass: string(item.SafetyClass), Error: problem,
		Summary: item.Summary, DroppedMessages: item.DroppedMessages,
	}
	if item.DroppedMessages < 0 {
		return transcript.Item{}, invalidArtifact(path+".droppedMessages", "must not be negative")
	}
	if len(item.Content) > 0 {
		out.Content = make([]transcript.ContentBlock, len(item.Content))
		for i, block := range item.Content {
			converted, err := canonicalContent(fmt.Sprintf("%s.content[%d]", path, i), block)
			if err != nil {
				return transcript.Item{}, err
			}
			out.Content[i] = converted
		}
	}
	if len(item.Steps) > 0 {
		out.Steps = make([]transcript.PlanStep, len(item.Steps))
		for i, step := range item.Steps {
			if !validPlanStepStatus(step.Status) {
				return transcript.Item{}, invalidArtifact(fmt.Sprintf("%s.steps[%d].status", path, i), "unknown value %q", step.Status)
			}
			out.Steps[i] = transcript.PlanStep{ID: step.ID, Title: step.Title, Status: string(step.Status)}
		}
	}
	if item.Question != nil {
		question, err := canonicalQuestion(path+".question", *item.Question)
		if err != nil {
			return transcript.Item{}, err
		}
		out.Question = &question
	}
	if kind == transcript.QuestionItem && out.Question == nil {
		return transcript.Item{}, invalidArtifact(path+".question", "is required for a question item")
	}
	if item.Tool != nil {
		tool, err := canonicalTool(path+".tool", *item.Tool)
		if err != nil {
			return transcript.Item{}, err
		}
		out.Tool = &tool
	}
	if kind == transcript.ToolCall && out.Tool == nil {
		return transcript.Item{}, invalidArtifact(path+".tool", "is required for a toolCall item")
	}
	if err := validateItemUnion(path, item, kind); err != nil {
		return transcript.Item{}, err
	}
	return out, nil
}

func validateItemUnion(path string, item protocol.Item, kind transcript.ItemKind) error {
	present := map[string]bool{
		"content": len(item.Content) != 0, "text": item.Text != "", "redacted": item.Redacted,
		"steps": len(item.Steps) != 0, "question": item.Question != nil, "tool": item.Tool != nil,
		"safetyClass": item.SafetyClass != "", "error": item.Error != nil,
		"summary": item.Summary != "", "droppedMessages": item.DroppedMessages != 0,
	}
	allowed := map[string]bool{}
	switch kind {
	case transcript.UserMessage, transcript.AgentMessage:
		allowed["content"] = true
	case transcript.Reasoning:
		allowed["text"], allowed["redacted"] = true, true
	case transcript.Plan:
		allowed["steps"] = true
	case transcript.QuestionItem:
		allowed["question"] = true
	case transcript.ToolCall:
		allowed["tool"], allowed["safetyClass"], allowed["error"] = true, true, true
	case transcript.Compaction:
		allowed["summary"], allowed["droppedMessages"] = true, true
	}
	for _, field := range []string{
		"content", "text", "redacted", "steps", "question", "tool",
		"safetyClass", "error", "summary", "droppedMessages",
	} {
		if present[field] && !allowed[field] {
			return invalidArtifact(path+"."+field, "is not valid for item type %q", item.Type)
		}
	}
	return nil
}

func canonicalItemStatus(path string, status protocol.ItemStatus) (transcript.ItemStatus, error) {
	switch status {
	case protocol.ItemStatusRunning:
		return transcript.ItemRunning, nil
	case protocol.ItemStatusCompleted:
		return transcript.ItemCompleted, nil
	case protocol.ItemStatusIncomplete:
		return transcript.ItemIncomplete, nil
	default:
		return 0, invalidArtifact(path, "unknown value %q", status)
	}
}

func canonicalItemKind(path string, kind protocol.ItemType) (transcript.ItemKind, error) {
	switch kind {
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
		return 0, invalidArtifact(path, "unknown value %q", kind)
	}
}

func canonicalContent(path string, block protocol.ContentBlock) (transcript.ContentBlock, error) {
	switch block.Type {
	case protocol.ContentBlockText:
		if block.Mime != "" || block.Data != "" {
			return transcript.ContentBlock{}, invalidArtifact(path, "text content cannot carry mime or data")
		}
		return transcript.ContentBlock{Kind: transcript.TextContent, Text: block.Text}, nil
	case protocol.ContentBlockImage:
		if block.Mime == "" || block.Data == "" {
			return transcript.ContentBlock{}, invalidArtifact(path, "image requires mime and data")
		}
		if block.Text != "" {
			return transcript.ContentBlock{}, invalidArtifact(path+".text", "is not valid for image content")
		}
		return transcript.ContentBlock{Kind: transcript.ImageContent, Mime: block.Mime, Data: block.Data}, nil
	default:
		return transcript.ContentBlock{}, invalidArtifact(path+".type", "unknown value %q", block.Type)
	}
}

func canonicalQuestion(path string, question protocol.Question) (transcript.Question, error) {
	fields := make([]transcript.QuestionField, len(question.Fields))
	seen := make(map[string]struct{}, len(question.Fields))
	for i, field := range question.Fields {
		fieldPath := fmt.Sprintf("%s.fields[%d]", path, i)
		if field.Name == "" {
			return transcript.Question{}, invalidArtifact(fieldPath+".name", "is required")
		}
		if _, duplicate := seen[field.Name]; duplicate {
			return transcript.Question{}, invalidArtifact(fieldPath+".name", "duplicate name %q", field.Name)
		}
		seen[field.Name] = struct{}{}
		var kind transcript.QuestionFieldKind
		switch field.Type {
		case protocol.QuestionFieldText:
			kind = transcript.QuestionText
			if len(field.Options) != 0 || field.Multiple {
				return transcript.Question{}, invalidArtifact(fieldPath, "text field cannot carry options or multiple")
			}
		case protocol.QuestionFieldChoice:
			kind = transcript.QuestionChoice
		default:
			return transcript.Question{}, invalidArtifact(fieldPath+".type", "unknown value %q", field.Type)
		}
		options := make([]transcript.QuestionOption, len(field.Options))
		for j, option := range field.Options {
			if option.Label == "" {
				return transcript.Question{}, invalidArtifact(fmt.Sprintf("%s.options[%d].label", fieldPath, j), "is required")
			}
			options[j] = transcript.QuestionOption{Label: option.Label, Description: option.Description, Preview: option.Preview}
		}
		fields[i] = transcript.QuestionField{
			Name: field.Name, Label: field.Label, Header: field.Header,
			Required: field.Required, Kind: kind, Options: options, Multiple: field.Multiple,
		}
	}
	return transcript.Question{Prompt: question.Prompt, Fields: fields}, nil
}

func canonicalTool(path string, tool protocol.ToolInvocation) (transcript.ToolInvocation, error) {
	if tool.Name == "" {
		return transcript.ToolInvocation{}, invalidArtifact(path+".name", "is required")
	}
	if tool.Arguments == nil {
		return transcript.ToolInvocation{}, invalidArtifact(path+".arguments", "must be a JSON object")
	}
	out := transcript.ToolInvocation{Name: tool.Name, Arguments: tool.Arguments}
	if tool.Result != nil {
		out.Result = &transcript.ToolResult{Kind: transcript.RawToolResult, Raw: tool.Result}
	}
	return out, nil
}

func canonicalProblem(path string, problem *protocol.ProblemData, channel protocol.ErrorChannel) (*transcript.Problem, error) {
	if problem == nil {
		return nil, nil
	}
	var kind transcript.ProblemKind
	switch problem.Type {
	case protocol.ProblemInternalError:
		kind = transcript.InternalProblem
	case protocol.ProblemRunLost:
		kind = transcript.RunLostProblem
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
	default:
		return nil, invalidArtifact(path+".type", "unknown value %q", problem.Type)
	}
	if problem.Channel != channel {
		return nil, invalidArtifact(path+".channel", "must be %q", channel)
	}
	if problem.RetryAfterSeconds < 0 {
		return nil, invalidArtifact(path+".retryAfterSeconds", "must not be negative")
	}
	if len(problem.Errors) != 0 {
		return nil, invalidArtifact(path+".errors", "field errors are not valid in transcript records")
	}
	scope := transcript.RunProblem
	if channel == protocol.ErrorChannelTool {
		scope = transcript.ToolProblem
	}
	return &transcript.Problem{
		Kind: kind, Scope: scope, Detail: problem.Detail, DocURL: problem.DocURL,
		Retryable: problem.Retryable, RetryAfterSeconds: problem.RetryAfterSeconds,
	}, nil
}

func validateSafetyClass(path string, class protocol.SafetyClass) error {
	switch class {
	case "", protocol.SafetyClassSafe, protocol.SafetyClassWrite, protocol.SafetyClassExec, protocol.SafetyClassNetwork:
		return nil
	default:
		return invalidArtifact(path, "unknown value %q", class)
	}
}

func validPlanStepStatus(status protocol.PlanStepStatus) bool {
	switch status {
	case protocol.PlanStepPending, protocol.PlanStepRunning, protocol.PlanStepCompleted, protocol.PlanStepFailed:
		return true
	default:
		return false
	}
}

func invalidArtifact(path, format string, args ...any) error {
	detail := fmt.Sprintf(format, args...)
	return fmt.Errorf("%w: %s: %s", protocol.ErrInvalidParams, path, detail)
}
