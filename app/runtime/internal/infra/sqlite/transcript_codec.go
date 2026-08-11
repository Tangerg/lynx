package sqlite

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

type transcriptItemPayload struct {
	Status          string                 `json:"status"`
	FinishedAt      int64                  `json:"finishedAt,omitempty"`
	Kind            string                 `json:"kind"`
	Content         []contentPayload       `json:"content,omitempty"`
	Text            string                 `json:"text,omitempty"`
	Redacted        bool                   `json:"redacted,omitempty"`
	Question        *questionPayload       `json:"question,omitempty"`
	Tool            *toolInvocationPayload `json:"tool,omitempty"`
	SafetyClass     string                 `json:"safetyClass,omitempty"`
	Failure         *toolFailurePayload    `json:"failure,omitempty"`
	Summary         string                 `json:"summary,omitempty"`
	DroppedMessages int                    `json:"droppedMessages,omitempty"`
}

type contentPayload struct {
	Kind      string `json:"kind"`
	Text      string `json:"text,omitempty"`
	MediaType string `json:"mediaType,omitempty"`
	Data      string `json:"data,omitempty"`
}

type questionPayload struct {
	Fields []questionFieldPayload `json:"fields"`
}

type questionFieldPayload struct {
	Prompt      string                  `json:"prompt"`
	Header      string                  `json:"header,omitempty"`
	Kind        string                  `json:"kind"`
	Options     []questionOptionPayload `json:"options,omitempty"`
	Multiple    bool                    `json:"multiple,omitempty"`
	AllowCustom bool                    `json:"allowCustom,omitempty"`
}

type questionOptionPayload struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Preview     string `json:"preview,omitempty"`
}

type toolInvocationPayload struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	Result    json.RawMessage `json:"result,omitempty"`
}

type toolFailurePayload struct {
	Kind              string `json:"kind"`
	Scope             string `json:"scope"`
	Detail            string `json:"detail,omitempty"`
	DocURL            string `json:"docUrl,omitempty"`
	RetryAfterSeconds int    `json:"retryAfterSeconds,omitempty"`
}

func encodeTranscriptItem(item transcript.Item) ([]byte, error) {
	status, err := encodeItemStatus(item.Status())
	if err != nil {
		return nil, err
	}
	kind, err := encodeItemKind(item.Kind())
	if err != nil {
		return nil, err
	}
	payload := transcriptItemPayload{
		Status: status, Kind: kind, Text: item.Text(), Redacted: item.Redacted(),
		SafetyClass: string(item.SafetyClass()), Summary: item.Summary(),
		DroppedMessages: item.DroppedMessages(),
	}
	if !item.FinishedAt().IsZero() {
		payload.FinishedAt = item.FinishedAt().UnixNano()
	}
	content := item.Content()
	if len(content) > 0 {
		payload.Content = make([]contentPayload, len(content))
		for index, block := range content {
			encoded, err := encodeContentPayload(block)
			if err != nil {
				return nil, fmt.Errorf("content %d: %w", index, err)
			}
			payload.Content[index] = encoded
		}
	}
	if question, present := item.Question(); present {
		encoded, err := encodeQuestionPayload(question)
		if err != nil {
			return nil, err
		}
		payload.Question = &encoded
	}
	if invocation, present := item.ToolInvocation(); present {
		encoded := encodeToolInvocationPayload(invocation)
		payload.Tool = &encoded
	}
	if failure, present := item.Failure(); present {
		encoded, err := encodeToolFailurePayload(failure)
		if err != nil {
			return nil, err
		}
		payload.Failure = &encoded
	}
	return json.Marshal(payload)
}

func decodeTranscriptItem(data []byte) (transcript.ItemSnapshot, error) {
	var payload transcriptItemPayload
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return transcript.ItemSnapshot{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return transcript.ItemSnapshot{}, err
	}
	status, err := decodeItemStatus(payload.Status)
	if err != nil {
		return transcript.ItemSnapshot{}, err
	}
	kind, err := decodeItemKind(payload.Kind)
	if err != nil {
		return transcript.ItemSnapshot{}, err
	}
	snapshot := transcript.ItemSnapshot{
		Status: status, Kind: kind, Text: payload.Text, Redacted: payload.Redacted,
		SafetyClass: tool.SafetyClass(payload.SafetyClass), Summary: payload.Summary,
		DroppedMessages: payload.DroppedMessages,
	}
	if payload.FinishedAt != 0 {
		snapshot.FinishedAt = time.Unix(0, payload.FinishedAt).UTC()
	}
	if len(payload.Content) > 0 {
		snapshot.Content = make([]transcript.ContentBlock, len(payload.Content))
		for index, encoded := range payload.Content {
			block, err := decodeContentPayload(encoded)
			if err != nil {
				return transcript.ItemSnapshot{}, fmt.Errorf("content %d: %w", index, err)
			}
			snapshot.Content[index] = block
		}
	}
	if payload.Question != nil {
		question, err := decodeQuestionPayload(*payload.Question)
		if err != nil {
			return transcript.ItemSnapshot{}, err
		}
		snapshot.Question = &question
	}
	if payload.Tool != nil {
		invocation, err := decodeToolInvocationPayload(*payload.Tool)
		if err != nil {
			return transcript.ItemSnapshot{}, err
		}
		snapshot.Tool = &invocation
	}
	if payload.Failure != nil {
		failure, err := decodeToolFailurePayload(*payload.Failure)
		if err != nil {
			return transcript.ItemSnapshot{}, err
		}
		snapshot.Failure = &failure
	}
	return snapshot, nil
}

func encodeItemStatus(status transcript.ItemStatus) (string, error) {
	switch status {
	case transcript.ItemRunning:
		return "running", nil
	case transcript.ItemCompleted:
		return "completed", nil
	case transcript.ItemIncomplete:
		return "incomplete", nil
	default:
		return "", fmt.Errorf("unknown item status %d", status)
	}
}

func decodeItemStatus(status string) (transcript.ItemStatus, error) {
	switch status {
	case "running":
		return transcript.ItemRunning, nil
	case "completed":
		return transcript.ItemCompleted, nil
	case "incomplete":
		return transcript.ItemIncomplete, nil
	default:
		return 0, fmt.Errorf("unknown item status %q", status)
	}
}

func encodeItemKind(kind transcript.ItemKind) (string, error) {
	switch kind {
	case transcript.UserMessage:
		return "user_message", nil
	case transcript.AgentMessage:
		return "agent_message", nil
	case transcript.Reasoning:
		return "reasoning", nil
	case transcript.QuestionItem:
		return "question", nil
	case transcript.ToolCall:
		return "tool_call", nil
	case transcript.Compaction:
		return "compaction", nil
	default:
		return "", fmt.Errorf("unknown item kind %d", kind)
	}
}

func decodeItemKind(kind string) (transcript.ItemKind, error) {
	switch kind {
	case "user_message":
		return transcript.UserMessage, nil
	case "agent_message":
		return transcript.AgentMessage, nil
	case "reasoning":
		return transcript.Reasoning, nil
	case "question":
		return transcript.QuestionItem, nil
	case "tool_call":
		return transcript.ToolCall, nil
	case "compaction":
		return transcript.Compaction, nil
	default:
		return 0, fmt.Errorf("unknown item kind %q", kind)
	}
}

func encodeContentPayload(block transcript.ContentBlock) (contentPayload, error) {
	switch block.Kind {
	case transcript.TextContent:
		return contentPayload{Kind: "text", Text: block.Text}, nil
	case transcript.ImageContent:
		return contentPayload{
			Kind: "image", MediaType: block.MediaType,
			Data: base64.StdEncoding.EncodeToString(block.Bytes),
		}, nil
	default:
		return contentPayload{}, fmt.Errorf("unknown content kind %d", block.Kind)
	}
}

func decodeContentPayload(payload contentPayload) (transcript.ContentBlock, error) {
	switch payload.Kind {
	case "text":
		return transcript.ContentBlock{Kind: transcript.TextContent, Text: payload.Text}, nil
	case "image":
		data, err := base64.StdEncoding.DecodeString(payload.Data)
		if err != nil {
			return transcript.ContentBlock{}, fmt.Errorf("decode image data: %w", err)
		}
		return transcript.ContentBlock{
			Kind: transcript.ImageContent, MediaType: payload.MediaType, Bytes: data,
		}, nil
	default:
		return transcript.ContentBlock{}, fmt.Errorf("unknown content kind %q", payload.Kind)
	}
}

func encodeQuestionPayload(question transcript.Question) (questionPayload, error) {
	encoded := questionPayload{Fields: make([]questionFieldPayload, len(question.Fields))}
	for index, field := range question.Fields {
		var kind string
		switch field.Kind {
		case transcript.QuestionText:
			kind = "text"
		case transcript.QuestionChoice:
			kind = "choice"
		default:
			return questionPayload{}, fmt.Errorf("question field %d has unknown kind %d", index, field.Kind)
		}
		encodedField := questionFieldPayload{
			Prompt: field.Prompt, Header: field.Header, Kind: kind,
			Multiple: field.Multiple, AllowCustom: field.AllowCustom,
		}
		if len(field.Options) > 0 {
			encodedField.Options = make([]questionOptionPayload, len(field.Options))
			for optionIndex, option := range field.Options {
				encodedField.Options[optionIndex] = questionOptionPayload{
					Label: option.Label, Description: option.Description, Preview: option.Preview,
				}
			}
		}
		encoded.Fields[index] = encodedField
	}
	return encoded, nil
}

func decodeQuestionPayload(payload questionPayload) (transcript.Question, error) {
	question := transcript.Question{Fields: make([]transcript.QuestionField, len(payload.Fields))}
	for index, field := range payload.Fields {
		var kind transcript.QuestionFieldKind
		switch field.Kind {
		case "text":
			kind = transcript.QuestionText
		case "choice":
			kind = transcript.QuestionChoice
		default:
			return transcript.Question{}, fmt.Errorf("question field %d has unknown kind %q", index, field.Kind)
		}
		decoded := transcript.QuestionField{
			Prompt: field.Prompt, Header: field.Header, Kind: kind,
			Multiple: field.Multiple, AllowCustom: field.AllowCustom,
		}
		if len(field.Options) > 0 {
			decoded.Options = make([]transcript.QuestionOption, len(field.Options))
			for optionIndex, option := range field.Options {
				decoded.Options[optionIndex] = transcript.QuestionOption{
					Label: option.Label, Description: option.Description, Preview: option.Preview,
				}
			}
		}
		question.Fields[index] = decoded
	}
	return question, nil
}

func encodeToolInvocationPayload(invocation transcript.ToolInvocation) toolInvocationPayload {
	payload := toolInvocationPayload{
		Name: invocation.Name, Arguments: json.RawMessage(invocation.Arguments.Canonical()),
	}
	if invocation.Result != nil {
		payload.Result = json.RawMessage(invocation.Result.Canonical())
	}
	return payload
}

func decodeToolInvocationPayload(payload toolInvocationPayload) (transcript.ToolInvocation, error) {
	arguments, err := tool.ParseArguments(string(payload.Arguments))
	if err != nil {
		return transcript.ToolInvocation{}, fmt.Errorf("tool arguments: %w", err)
	}
	invocation := transcript.ToolInvocation{Name: payload.Name, Arguments: arguments}
	if len(payload.Result) > 0 {
		result, err := tool.ParseResult(payload.Result)
		if err != nil {
			return transcript.ToolInvocation{}, fmt.Errorf("tool result: %w", err)
		}
		invocation.Result = &result
	}
	return invocation, nil
}

func encodeToolFailurePayload(failure tool.Failure) (toolFailurePayload, error) {
	kind, err := encodeToolFailureKind(failure.Kind)
	if err != nil {
		return toolFailurePayload{}, err
	}
	return toolFailurePayload{
		Kind: kind, Scope: "tool", Detail: failure.Detail, DocURL: failure.DocURL,
		RetryAfterSeconds: int(failure.RetryAfter / time.Second),
	}, nil
}

func decodeToolFailurePayload(payload toolFailurePayload) (tool.Failure, error) {
	kind, err := decodeToolFailureKind(payload.Kind)
	if err != nil {
		return tool.Failure{}, err
	}
	if payload.Scope != "tool" {
		return tool.Failure{}, fmt.Errorf("unknown Tool failure scope %q", payload.Scope)
	}
	return tool.Failure{
		Kind: kind, Detail: payload.Detail, DocURL: payload.DocURL,
		RetryAfter: time.Duration(payload.RetryAfterSeconds) * time.Second,
	}, nil
}

func encodeToolFailureKind(kind tool.FailureKind) (string, error) {
	switch kind {
	case tool.FailureInternal:
		return "internal", nil
	case tool.FailureDenied:
		return "denied_by_user", nil
	case tool.FailureExecution:
		return "tool_failed", nil
	case tool.FailureChildRunCanceled:
		return "child_run_canceled", nil
	case tool.FailureCanceled:
		return "tool_canceled", nil
	default:
		return "", fmt.Errorf("unknown Tool failure kind %d", kind)
	}
}

func decodeToolFailureKind(kind string) (tool.FailureKind, error) {
	switch kind {
	case "internal":
		return tool.FailureInternal, nil
	case "denied_by_user":
		return tool.FailureDenied, nil
	case "tool_failed":
		return tool.FailureExecution, nil
	case "child_run_canceled":
		return tool.FailureChildRunCanceled, nil
	case "tool_canceled":
		return tool.FailureCanceled, nil
	default:
		return 0, fmt.Errorf("unknown Tool failure kind %q", kind)
	}
}
