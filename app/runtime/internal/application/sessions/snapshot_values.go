package sessions

import (
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

func validateSnapshotUsage(usage *transcript.Usage) error {
	if usage == nil {
		return nil
	}
	if err := validateSnapshotModelUsage(usage.ModelUsage); err != nil {
		return err
	}
	for model, perModel := range usage.ByModel {
		if model == "" {
			return errors.New("contains an empty model id")
		}
		if err := validateSnapshotModelUsage(perModel); err != nil {
			return fmt.Errorf("model %q: %w", model, err)
		}
	}
	return nil
}

func validateSnapshotModelUsage(usage transcript.ModelUsage) error {
	if usage.InputTokens < 0 || usage.OutputTokens < 0 || usage.CacheReadTokens < 0 || usage.CacheWriteTokens < 0 || usage.ReasoningTokens < 0 {
		return errors.New("token counts must not be negative")
	}
	if usage.CostUSD != nil && *usage.CostUSD < 0 {
		return errors.New("cost must not be negative")
	}
	return nil
}

func validateSnapshotProblem(problem *transcript.Problem, scope transcript.ProblemScope) error {
	if problem == nil {
		return nil
	}
	if problem.Scope != scope {
		return fmt.Errorf("scope %d, want %d", problem.Scope, scope)
	}
	switch problem.Kind {
	case transcript.InternalProblem, transcript.RunLostProblem, transcript.AgentStuckProblem,
		transcript.RateLimitedProblem, transcript.InvalidAPIKeyProblem, transcript.TimeoutProblem,
		transcript.ProviderUnavailableProblem, transcript.ProviderRejectedProblem,
		transcript.DeniedByUserProblem, transcript.ToolFailedProblem:
	default:
		return fmt.Errorf("unknown kind %d", problem.Kind)
	}
	if problem.RetryAfterSeconds < 0 {
		return errors.New("retry delay must not be negative")
	}
	return nil
}

func validateSnapshotItem(item transcript.Item) error {
	if item.DroppedMessages < 0 {
		return errors.New("dropped messages must not be negative")
	}
	if err := validateSnapshotProblem(item.Error, transcript.ToolProblem); err != nil {
		return fmt.Errorf("tool problem: %w", err)
	}
	for index, block := range item.Content {
		if err := validateSnapshotContent(block); err != nil {
			return fmt.Errorf("content %d: %w", index, err)
		}
	}
	for index, step := range item.Steps {
		if !validSnapshotPlanStepStatus(step.Status) {
			return fmt.Errorf("plan step %d has unknown status %q", index, step.Status)
		}
	}
	if item.Question != nil {
		if err := validateSnapshotQuestion(*item.Question); err != nil {
			return err
		}
	}
	if item.Tool != nil {
		if item.Tool.Name == "" {
			return errors.New("tool name is required")
		}
	}

	present := map[string]bool{
		"content": len(item.Content) != 0, "text": item.Text != "", "redacted": item.Redacted,
		"steps": len(item.Steps) != 0, "question": item.Question != nil, "tool": item.Tool != nil,
		"safetyClass": item.SafetyClass != "", "error": item.Error != nil,
		"summary": item.Summary != "", "droppedMessages": item.DroppedMessages != 0,
	}
	allowed := map[string]bool{}
	switch item.Kind {
	case transcript.UserMessage, transcript.AgentMessage:
		allowed["content"] = true
	case transcript.Reasoning:
		allowed["text"], allowed["redacted"] = true, true
	case transcript.Plan:
		allowed["steps"] = true
	case transcript.QuestionItem:
		allowed["question"] = true
		if item.Question == nil {
			return errors.New("question is required")
		}
	case transcript.ToolCall:
		allowed["tool"], allowed["safetyClass"], allowed["error"] = true, true, true
		if item.Tool == nil {
			return errors.New("tool invocation is required")
		}
		if item.SafetyClass != "" && !item.SafetyClass.Valid() {
			return fmt.Errorf("unknown safety class %q", item.SafetyClass)
		}
	case transcript.Compaction:
		allowed["summary"], allowed["droppedMessages"] = true, true
	default:
		return fmt.Errorf("unknown kind %d", item.Kind)
	}
	for field, exists := range present {
		if exists && !allowed[field] {
			return fmt.Errorf("%s is not valid for item kind %d", field, item.Kind)
		}
	}
	return nil
}

func validateSnapshotContent(block transcript.ContentBlock) error {
	switch block.Kind {
	case transcript.TextContent:
		if block.Mime != "" || block.Data != "" {
			return errors.New("text content cannot carry mime or data")
		}
	case transcript.ImageContent:
		if block.Mime == "" || block.Data == "" {
			return errors.New("image content requires mime and data")
		}
		if block.Text != "" {
			return errors.New("image content cannot carry text")
		}
	default:
		return fmt.Errorf("unknown content kind %d", block.Kind)
	}
	return nil
}

func validateSnapshotQuestion(question transcript.Question) error {
	seen := make(map[string]struct{}, len(question.Fields))
	for index, field := range question.Fields {
		if field.Name == "" {
			return fmt.Errorf("question field %d name is required", index)
		}
		if _, duplicate := seen[field.Name]; duplicate {
			return fmt.Errorf("question field %q is duplicated", field.Name)
		}
		seen[field.Name] = struct{}{}
		switch field.Kind {
		case transcript.QuestionText:
			if len(field.Options) != 0 || field.Multiple {
				return fmt.Errorf("text question field %q cannot carry options or multiple", field.Name)
			}
		case transcript.QuestionChoice:
		default:
			return fmt.Errorf("question field %q has unknown kind %d", field.Name, field.Kind)
		}
		for optionIndex, option := range field.Options {
			if option.Label == "" {
				return fmt.Errorf("question field %q option %d label is required", field.Name, optionIndex)
			}
		}
	}
	return nil
}

func validSnapshotPlanStepStatus(status string) bool {
	switch status {
	case "pending", "running", "completed", "failed":
		return true
	default:
		return false
	}
}
