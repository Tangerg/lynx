package server

import (
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

func presentItem(item transcript.Item) protocol.Item {
	out := protocol.Item{
		ID: item.ID, RunID: item.RunID, Status: presentItemStatus(item.Status),
		CreatedAt: item.CreatedAt, Type: presentItemKind(item.Kind),
		Text: item.Text, Redacted: item.Redacted,
		SafetyClass: protocol.SafetyClass(item.SafetyClass), Error: presentProblem(item.Error),
		Summary: item.Summary, DroppedMessages: item.DroppedMessages,
	}
	if len(item.Content) > 0 {
		out.Content = make([]protocol.ContentBlock, len(item.Content))
		for i, block := range item.Content {
			out.Content[i] = presentContent(block)
		}
	}
	if len(item.Steps) > 0 {
		out.Steps = presentPlanSteps(item.Steps)
	}
	if item.Question != nil {
		question := presentQuestion(*item.Question)
		out.Question = &question
	}
	if item.Tool != nil {
		tool := presentTool(*item.Tool)
		out.Tool = &tool
	}
	return out
}

func presentItemStatus(status transcript.ItemStatus) protocol.ItemStatus {
	switch status {
	case transcript.ItemCompleted:
		return protocol.ItemStatusCompleted
	case transcript.ItemIncomplete:
		return protocol.ItemStatusIncomplete
	default:
		return protocol.ItemStatusRunning
	}
}

func presentItemKind(kind transcript.ItemKind) protocol.ItemType {
	switch kind {
	case transcript.AgentMessage:
		return protocol.ItemTypeAgentMessage
	case transcript.Reasoning:
		return protocol.ItemTypeReasoning
	case transcript.Plan:
		return protocol.ItemTypePlan
	case transcript.QuestionItem:
		return protocol.ItemTypeQuestion
	case transcript.ToolCall:
		return protocol.ItemTypeToolCall
	case transcript.Compaction:
		return protocol.ItemTypeCompaction
	default:
		return protocol.ItemTypeUserMessage
	}
}

func presentContent(block transcript.ContentBlock) protocol.ContentBlock {
	kind := protocol.ContentBlockText
	if block.Kind == transcript.ImageContent {
		kind = protocol.ContentBlockImage
	}
	return protocol.ContentBlock{Type: kind, Text: block.Text, Mime: block.Mime, Data: block.Data}
}

func presentPlanSteps(steps []transcript.PlanStep) []protocol.PlanStep {
	out := make([]protocol.PlanStep, len(steps))
	for i, step := range steps {
		out[i] = protocol.PlanStep{ID: step.ID, Title: step.Title, Status: protocol.PlanStepStatus(step.Status)}
	}
	return out
}

func presentQuestion(question transcript.Question) protocol.Question {
	fields := make([]protocol.QuestionField, len(question.Fields))
	for i, field := range question.Fields {
		kind := protocol.QuestionFieldText
		if field.Kind == transcript.QuestionChoice {
			kind = protocol.QuestionFieldChoice
		}
		options := make([]protocol.QuestionOption, len(field.Options))
		for j, option := range field.Options {
			options[j] = protocol.QuestionOption{
				Label: option.Label, Description: option.Description, Preview: option.Preview,
			}
		}
		fields[i] = protocol.QuestionField{
			Name: field.Name, Label: field.Label, Header: field.Header,
			Required: field.Required, Type: kind, Options: options, Multiple: field.Multiple,
		}
	}
	return protocol.Question{Prompt: question.Prompt, Fields: fields}
}

func presentTool(tool transcript.ToolInvocation) protocol.ToolInvocation {
	return protocol.ToolInvocation{
		Name: tool.Name, Arguments: tool.Arguments, Result: presentToolResult(tool),
	}
}

func presentDelta(delta runs.ItemDelta) protocol.ItemDelta {
	kind := protocol.DeltaContent
	switch delta.Kind {
	case runs.ReasoningDeltaKind:
		kind = protocol.DeltaReasoning
	case runs.ToolArgumentsDelta:
		kind = protocol.DeltaToolArguments
	case runs.ToolOutputDelta:
		kind = protocol.DeltaToolOutput
	case runs.PlanDelta:
		kind = protocol.DeltaPlan
	}
	return protocol.ItemDelta{
		Type: kind, Index: delta.Index, Text: delta.Text,
		ArgumentsTextDelta: delta.ArgumentsTextDelta, Steps: presentPlanSteps(delta.Steps),
	}
}
