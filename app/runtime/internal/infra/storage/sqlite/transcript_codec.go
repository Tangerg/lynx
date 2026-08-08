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
	Problem         *problemPayload        `json:"problem,omitempty"`
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

type problemPayload struct {
	Kind              string `json:"kind"`
	Scope             string `json:"scope"`
	Detail            string `json:"detail,omitempty"`
	DocURL            string `json:"docUrl,omitempty"`
	RetryAfterSeconds int    `json:"retryAfterSeconds,omitempty"`
}

func encodeTranscriptItem(item transcript.Item) ([]byte, error) {
	status, err := encodeItemStatus(item.Status)
	if err != nil {
		return nil, err
	}
	kind, err := encodeItemKind(item.Kind)
	if err != nil {
		return nil, err
	}
	payload := transcriptItemPayload{
		Status: status, Kind: kind, Text: item.Text, Redacted: item.Redacted,
		SafetyClass: string(item.SafetyClass), Summary: item.Summary,
		DroppedMessages: item.DroppedMessages,
	}
	if !item.FinishedAt.IsZero() {
		payload.FinishedAt = item.FinishedAt.UnixNano()
	}
	if len(item.Content) > 0 {
		payload.Content = make([]contentPayload, len(item.Content))
		for index, block := range item.Content {
			encoded, err := encodeContentPayload(block)
			if err != nil {
				return nil, fmt.Errorf("content %d: %w", index, err)
			}
			payload.Content[index] = encoded
		}
	}
	if item.Question != nil {
		encoded, err := encodeQuestionPayload(*item.Question)
		if err != nil {
			return nil, err
		}
		payload.Question = &encoded
	}
	if item.Tool != nil {
		encoded := encodeToolInvocationPayload(*item.Tool)
		payload.Tool = &encoded
	}
	if item.Error != nil {
		encoded, err := encodeProblemPayload(*item.Error)
		if err != nil {
			return nil, err
		}
		payload.Problem = &encoded
	}
	return json.Marshal(payload)
}

func decodeTranscriptItem(data []byte) (transcript.Item, error) {
	var payload transcriptItemPayload
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return transcript.Item{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return transcript.Item{}, err
	}
	status, err := decodeItemStatus(payload.Status)
	if err != nil {
		return transcript.Item{}, err
	}
	kind, err := decodeItemKind(payload.Kind)
	if err != nil {
		return transcript.Item{}, err
	}
	item := transcript.Item{
		Status: status, Kind: kind, Text: payload.Text, Redacted: payload.Redacted,
		SafetyClass: tool.SafetyClass(payload.SafetyClass), Summary: payload.Summary,
		DroppedMessages: payload.DroppedMessages,
	}
	if payload.FinishedAt != 0 {
		item.FinishedAt = time.Unix(0, payload.FinishedAt).UTC()
	}
	if len(payload.Content) > 0 {
		item.Content = make([]transcript.ContentBlock, len(payload.Content))
		for index, encoded := range payload.Content {
			block, err := decodeContentPayload(encoded)
			if err != nil {
				return transcript.Item{}, fmt.Errorf("content %d: %w", index, err)
			}
			item.Content[index] = block
		}
	}
	if payload.Question != nil {
		question, err := decodeQuestionPayload(*payload.Question)
		if err != nil {
			return transcript.Item{}, err
		}
		item.Question = &question
	}
	if payload.Tool != nil {
		invocation, err := decodeToolInvocationPayload(*payload.Tool)
		if err != nil {
			return transcript.Item{}, err
		}
		item.Tool = &invocation
	}
	if payload.Problem != nil {
		problem, err := decodeProblemPayload(*payload.Problem)
		if err != nil {
			return transcript.Item{}, err
		}
		item.Error = &problem
	}
	return item, nil
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

func encodeProblemPayload(problem transcript.Problem) (problemPayload, error) {
	kind, err := encodeProblemKind(problem.Kind)
	if err != nil {
		return problemPayload{}, err
	}
	var scope string
	switch problem.Scope {
	case transcript.RunProblem:
		scope = "run"
	case transcript.ToolProblem:
		scope = "tool"
	default:
		return problemPayload{}, fmt.Errorf("unknown problem scope %d", problem.Scope)
	}
	return problemPayload{
		Kind: kind, Scope: scope, Detail: problem.Detail, DocURL: problem.DocURL,
		RetryAfterSeconds: problem.RetryAfterSeconds,
	}, nil
}

func decodeProblemPayload(payload problemPayload) (transcript.Problem, error) {
	kind, err := decodeProblemKind(payload.Kind)
	if err != nil {
		return transcript.Problem{}, err
	}
	var scope transcript.ProblemScope
	switch payload.Scope {
	case "run":
		scope = transcript.RunProblem
	case "tool":
		scope = transcript.ToolProblem
	default:
		return transcript.Problem{}, fmt.Errorf("unknown problem scope %q", payload.Scope)
	}
	return transcript.Problem{
		Kind: kind, Scope: scope, Detail: payload.Detail, DocURL: payload.DocURL,
		RetryAfterSeconds: payload.RetryAfterSeconds,
	}, nil
}

func encodeProblemKind(kind transcript.ProblemKind) (string, error) {
	switch kind {
	case transcript.InternalProblem:
		return "internal", nil
	case transcript.RunLostProblem:
		return "run_lost", nil
	case transcript.AgentStuckProblem:
		return "agent_stuck", nil
	case transcript.RateLimitedProblem:
		return "rate_limited", nil
	case transcript.InvalidAPIKeyProblem:
		return "invalid_api_key", nil
	case transcript.TimeoutProblem:
		return "timeout", nil
	case transcript.ProviderUnavailableProblem:
		return "provider_unavailable", nil
	case transcript.ProviderRejectedProblem:
		return "provider_rejected", nil
	case transcript.DeniedByUserProblem:
		return "denied_by_user", nil
	case transcript.ToolFailedProblem:
		return "tool_failed", nil
	case transcript.ChildRunCanceledProblem:
		return "child_run_canceled", nil
	default:
		return "", fmt.Errorf("unknown problem kind %d", kind)
	}
}

func decodeProblemKind(kind string) (transcript.ProblemKind, error) {
	switch kind {
	case "internal":
		return transcript.InternalProblem, nil
	case "run_lost":
		return transcript.RunLostProblem, nil
	case "agent_stuck":
		return transcript.AgentStuckProblem, nil
	case "rate_limited":
		return transcript.RateLimitedProblem, nil
	case "invalid_api_key":
		return transcript.InvalidAPIKeyProblem, nil
	case "timeout":
		return transcript.TimeoutProblem, nil
	case "provider_unavailable":
		return transcript.ProviderUnavailableProblem, nil
	case "provider_rejected":
		return transcript.ProviderRejectedProblem, nil
	case "denied_by_user":
		return transcript.DeniedByUserProblem, nil
	case "tool_failed":
		return transcript.ToolFailedProblem, nil
	case "child_run_canceled":
		return transcript.ChildRunCanceledProblem, nil
	default:
		return 0, fmt.Errorf("unknown problem kind %q", kind)
	}
}
