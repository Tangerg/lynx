package server

import (
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

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
	safetyClass, err := canonicalSafetyClass(path+".safetyClass", item.SafetyClass)
	if err != nil {
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
		SafetyClass: safetyClass, Error: problem,
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
	return transcript.ToolInvocation{
		Name: tool.Name, Arguments: tool.Arguments, Result: tool.Result,
	}, nil
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

func canonicalSafetyClass(path string, class protocol.SafetyClass) (tool.SafetyClass, error) {
	if class == "" {
		return "", nil
	}
	canonical := tool.SafetyClass(class)
	if !canonical.Valid() {
		return "", invalidArtifact(path, "unknown value %q", class)
	}
	return canonical, nil
}

func validPlanStepStatus(status protocol.PlanStepStatus) bool {
	switch status {
	case protocol.PlanStepPending, protocol.PlanStepRunning, protocol.PlanStepCompleted, protocol.PlanStepFailed:
		return true
	default:
		return false
	}
}
