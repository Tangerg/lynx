package sqlite

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

type transcriptItemPayload struct {
	Status                 transcript.ItemStatus   `json:"status"`
	FinishedAt             int64                   `json:"finishedAt,omitempty"`
	ExecutionDurationNanos *int64                  `json:"executionDurationNanos,omitempty"`
	Kind                   transcript.ItemKind     `json:"kind"`
	Phase                  transcript.MessagePhase `json:"phase,omitempty"`
	Content                []contentPayload        `json:"content,omitempty"`
	Text                   string                  `json:"text,omitempty"`
	Redacted               bool                    `json:"redacted,omitempty"`
	Question               *questionPayload        `json:"question,omitempty"`
	Tool                   *toolInvocationPayload  `json:"tool,omitempty"`
	SafetyClass            tool.SafetyClass        `json:"safetyClass,omitempty"`
	ApprovalDecision       approval.Decision       `json:"approvalDecision,omitempty"`
	Failure                *toolFailurePayload     `json:"failure,omitempty"`
	Summary                string                  `json:"summary,omitempty"`
	DroppedMessages        int                     `json:"droppedMessages,omitempty"`
}

type contentPayload struct {
	Kind      transcript.ContentKind `json:"kind"`
	Text      string                 `json:"text,omitempty"`
	MediaType string                 `json:"mediaType,omitempty"`
	Data      string                 `json:"data,omitempty"`
}

type questionPayload struct {
	Fields  []questionFieldPayload `json:"fields"`
	Answers [][]string             `json:"answers,omitempty"`
}

type questionFieldPayload struct {
	Prompt      string                       `json:"prompt"`
	Header      string                       `json:"header,omitempty"`
	Kind        transcript.QuestionFieldKind `json:"kind"`
	Options     []questionOptionPayload      `json:"options,omitempty"`
	Multiple    bool                         `json:"multiple,omitempty"`
	AllowCustom bool                         `json:"allowCustom,omitempty"`
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
	Kind              tool.FailureKind `json:"kind"`
	Scope             string           `json:"scope"`
	Detail            string           `json:"detail,omitempty"`
	DocURL            string           `json:"docUrl,omitempty"`
	RetryAfterSeconds int              `json:"retryAfterSeconds,omitempty"`
}

const toolFailureScope = "tool"

func encodeTranscriptItem(item transcript.Item) ([]byte, error) {
	if !item.Status().Valid() {
		return nil, fmt.Errorf("unknown item status %q", item.Status())
	}
	if !item.Kind().Valid() {
		return nil, fmt.Errorf("unknown item kind %q", item.Kind())
	}
	if phase := item.MessagePhase(); phase != transcript.MessagePhaseNone && !phase.Valid() {
		return nil, fmt.Errorf("unknown message phase %q", phase)
	}
	payload := transcriptItemPayload{
		Status: item.Status(), Kind: item.Kind(), Phase: item.MessagePhase(), Text: item.Text(), Redacted: item.Redacted(),
		SafetyClass: item.SafetyClass(), ApprovalDecision: item.ApprovalDecision(),
		Summary:         item.Summary(),
		DroppedMessages: item.DroppedMessages(),
	}
	if !item.FinishedAt().IsZero() {
		payload.FinishedAt = item.FinishedAt().UnixNano()
	}
	if duration, known := item.ExecutionDuration(); known {
		nanos := duration.Nanoseconds()
		payload.ExecutionDurationNanos = &nanos
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
	if !payload.Status.Valid() {
		return transcript.ItemSnapshot{}, fmt.Errorf("unknown item status %q", payload.Status)
	}
	if !payload.Kind.Valid() {
		return transcript.ItemSnapshot{}, fmt.Errorf("unknown item kind %q", payload.Kind)
	}
	if payload.Phase != transcript.MessagePhaseNone && !payload.Phase.Valid() {
		return transcript.ItemSnapshot{}, fmt.Errorf("unknown message phase %q", payload.Phase)
	}
	snapshot := transcript.ItemSnapshot{
		Status: payload.Status, Kind: payload.Kind, MessagePhase: payload.Phase, Text: payload.Text, Redacted: payload.Redacted,
		SafetyClass:      payload.SafetyClass,
		ApprovalDecision: payload.ApprovalDecision, Summary: payload.Summary,
		DroppedMessages: payload.DroppedMessages,
	}
	if payload.FinishedAt != 0 {
		snapshot.FinishedAt = time.Unix(0, payload.FinishedAt).UTC()
	}
	if payload.ExecutionDurationNanos != nil {
		duration := time.Duration(*payload.ExecutionDurationNanos)
		snapshot.ExecutionDuration = &duration
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

func encodeContentPayload(block transcript.ContentBlock) (contentPayload, error) {
	switch block.Kind {
	case transcript.TextContent:
		return contentPayload{Kind: block.Kind, Text: block.Text}, nil
	case transcript.ImageContent:
		return contentPayload{
			Kind: block.Kind, MediaType: block.MediaType,
			Data: base64.StdEncoding.EncodeToString(block.Bytes),
		}, nil
	default:
		return contentPayload{}, fmt.Errorf("unknown content kind %q", block.Kind)
	}
}

func decodeContentPayload(payload contentPayload) (transcript.ContentBlock, error) {
	switch payload.Kind {
	case transcript.TextContent:
		return transcript.ContentBlock{Kind: transcript.TextContent, Text: payload.Text}, nil
	case transcript.ImageContent:
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
	encoded := questionPayload{
		Fields:  make([]questionFieldPayload, len(question.Fields)),
		Answers: cloneStringMatrix(question.Answers),
	}
	for index, field := range question.Fields {
		if !field.Kind.Valid() {
			return questionPayload{}, fmt.Errorf("question field %d has unknown kind %q", index, field.Kind)
		}
		encodedField := questionFieldPayload{
			Prompt: field.Prompt, Header: field.Header, Kind: field.Kind,
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
	question := transcript.Question{
		Fields:  make([]transcript.QuestionField, len(payload.Fields)),
		Answers: cloneStringMatrix(payload.Answers),
	}
	for index, field := range payload.Fields {
		if !field.Kind.Valid() {
			return transcript.Question{}, fmt.Errorf("question field %d has unknown kind %q", index, field.Kind)
		}
		decoded := transcript.QuestionField{
			Prompt: field.Prompt, Header: field.Header, Kind: field.Kind,
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

func cloneStringMatrix(values [][]string) [][]string {
	if values == nil {
		return nil
	}
	cloned := make([][]string, len(values))
	for index, row := range values {
		cloned[index] = append([]string(nil), row...)
	}
	return cloned
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
	if !failure.Kind.Valid() {
		return toolFailurePayload{}, fmt.Errorf("unknown Tool failure kind %q", failure.Kind)
	}
	return toolFailurePayload{
		Kind: failure.Kind, Scope: toolFailureScope, Detail: failure.Detail, DocURL: failure.DocURL,
		RetryAfterSeconds: int(failure.RetryAfter / time.Second),
	}, nil
}

func decodeToolFailurePayload(payload toolFailurePayload) (tool.Failure, error) {
	if !payload.Kind.Valid() {
		return tool.Failure{}, fmt.Errorf("unknown Tool failure kind %q", payload.Kind)
	}
	if payload.Scope != toolFailureScope {
		return tool.Failure{}, fmt.Errorf("unknown Tool failure scope %q", payload.Scope)
	}
	return tool.Failure{
		Kind: payload.Kind, Detail: payload.Detail, DocURL: payload.DocURL,
		RetryAfter: time.Duration(payload.RetryAfterSeconds) * time.Second,
	}, nil
}
