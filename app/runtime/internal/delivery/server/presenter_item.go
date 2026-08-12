package server

import (
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func presentItem(item transcript.Item) protocol.Item {
	var failureRef *tool.Failure
	if failure, present := item.Failure(); present {
		failureRef = &failure
	}
	out := protocol.Item{
		ID: item.ID(), RunID: item.RunID(), Status: presentItemStatus(item.Status()),
		Type: presentItemKind(item.Kind()),
		Text: item.Text(), Redacted: item.Redacted(),
		SafetyClass: presentSafetyClass(item.SafetyClass()), Error: presentToolFailure(failureRef),
		Summary: item.Summary(), DroppedMessages: item.DroppedMessages(),
	}
	content := item.Content()
	if len(content) > 0 {
		out.Content = make([]protocol.ContentBlock, len(content))
		for i, block := range content {
			out.Content[i] = presentContent(block)
		}
	}
	if value, present := item.Question(); present {
		question := presentQuestion(value)
		out.Question = &question
	}
	if value, present := item.ToolInvocation(); present {
		tool := presentTool(value)
		out.Tool = &tool
	}
	if item.Kind() == transcript.ToolCall {
		out.StartedAt = item.OccurredAt()
		out.FinishedAt = item.FinishedAt()
		out.DurationMillis = presentToolDurationMillis(item)
	} else {
		out.CreatedAt = item.OccurredAt()
	}
	return out
}

func presentToolDurationMillis(item transcript.Item) *int64 {
	duration, known := item.ExecutionDuration()
	if !known {
		return nil
	}
	millis := duration.Milliseconds()
	return &millis
}

func presentItemStart(item runs.ItemStart) protocol.Item {
	out := protocol.Item{
		ID: item.ItemID, RunID: item.RunID, Status: protocol.ItemStatusRunning,
		Type: presentItemKind(item.Kind), SafetyClass: presentSafetyClass(item.SafetyClass),
	}
	if item.Kind == transcript.ToolCall {
		out.StartedAt = item.OccurredAt
		if item.ToolInvocation != nil {
			tool := presentTool(*item.ToolInvocation)
			out.Tool = &tool
		}
	} else {
		out.CreatedAt = item.OccurredAt
	}
	return out
}

func presentItemStatus(status transcript.ItemStatus) protocol.ItemStatus {
	switch status {
	case transcript.ItemRunning:
		return protocol.ItemStatusRunning
	case transcript.ItemCompleted:
		return protocol.ItemStatusCompleted
	case transcript.ItemIncomplete:
		return protocol.ItemStatusIncomplete
	default:
		panic("server: unknown transcript item status")
	}
}

func presentItemKind(kind transcript.ItemKind) protocol.ItemType {
	switch kind {
	case transcript.UserMessage:
		return protocol.ItemTypeUserMessage
	case transcript.AgentMessage:
		return protocol.ItemTypeAgentMessage
	case transcript.Reasoning:
		return protocol.ItemTypeReasoning
	case transcript.QuestionItem:
		return protocol.ItemTypeQuestion
	case transcript.ToolCall:
		return protocol.ItemTypeToolCall
	case transcript.Compaction:
		return protocol.ItemTypeCompaction
	default:
		panic("server: unknown transcript item kind")
	}
}

func presentContent(block transcript.ContentBlock) protocol.ContentBlock {
	encoded, err := encodeContent(block)
	if err != nil {
		panic("server: " + err.Error())
	}
	return protocol.ContentBlock{Type: encoded.kind, Text: encoded.text, Mime: encoded.mime, Data: encoded.data}
}

func presentQuestion(question transcript.Question) protocol.Question {
	fields := make([]protocol.QuestionField, len(question.Fields))
	for i, field := range question.Fields {
		var kind protocol.QuestionFieldType
		switch field.Kind {
		case transcript.QuestionText:
			kind = protocol.QuestionFieldText
		case transcript.QuestionChoice:
			kind = protocol.QuestionFieldChoice
		default:
			panic("server: unknown transcript question-field kind")
		}
		var options []protocol.QuestionOption
		if len(field.Options) > 0 {
			options = make([]protocol.QuestionOption, len(field.Options))
			for j, option := range field.Options {
				options[j] = protocol.QuestionOption{
					Label: option.Label, Description: option.Description, Preview: option.Preview,
				}
			}
		}
		fields[i] = protocol.QuestionField{
			Prompt: field.Prompt, Header: field.Header, Type: kind,
			Options: options, Multiple: field.Multiple, AllowCustom: field.AllowCustom,
		}
	}
	return protocol.Question{Fields: fields, Answers: cloneAnswers(question.Answers)}
}

func cloneAnswers(answers [][]string) [][]string {
	if answers == nil {
		return nil
	}
	cloned := make([][]string, len(answers))
	for index, values := range answers {
		cloned[index] = append([]string(nil), values...)
	}
	return cloned
}

func presentTool(tool transcript.ToolInvocation) protocol.ToolInvocation {
	var result any
	if tool.Result != nil {
		result = tool.Result.Any()
	}
	return protocol.ToolInvocation{
		Name: tool.Name, Arguments: tool.Arguments.Map(), Result: result,
	}
}

func presentDelta(delta runs.ItemDelta) protocol.ItemDelta {
	var kind protocol.ItemDeltaType
	switch delta.Kind {
	case runs.ContentDelta:
		kind = protocol.DeltaContent
	case runs.ReasoningDeltaKind:
		kind = protocol.DeltaReasoning
	case runs.ToolArgumentsDelta:
		kind = protocol.DeltaToolArguments
	case runs.ToolOutputDelta:
		kind = protocol.DeltaToolOutput
	default:
		panic("server: unknown item delta kind")
	}
	return protocol.ItemDelta{
		Type: kind, Index: delta.Index, Text: delta.Text,
		ArgumentsTextDelta: delta.ArgumentsTextDelta,
	}
}
