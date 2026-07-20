package toolloop_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/agent/toolloop"
	"github.com/Tangerg/lynx/core/chat"
)

func TestEventJSONRoundTrip(t *testing.T) {
	request := protocolRequest(t)
	response := &chat.Response{Choices: []chat.Choice{{
		Index:   0,
		Message: messagePointer(chat.NewAssistantMessage(chat.NewTextPart("done"))),
	}}}
	call := &chat.ToolCall{ID: "call-1", Name: "lookup", Arguments: `{}`}
	result := &chat.ToolResult{ID: "call-1", Name: "lookup", Result: "ok"}
	checkpoint := protocolCheckpoint(t)
	prompt := json.RawMessage(`"approve?"`)
	schema := json.RawMessage(`{"type":"string"}`)

	events := []toolloop.Event{
		{Kind: toolloop.EventModelRequest, Round: 1, Request: request},
		{Kind: toolloop.EventModelResponse, Round: 1, Response: response},
		{Kind: toolloop.EventToolCall, Round: 1, ToolCall: call},
		{Kind: toolloop.EventToolResult, Round: 1, ToolResult: result},
		{Kind: toolloop.EventPause, Round: 1, Pause: &toolloop.Pause{ID: "approval-1", Reason: "approval required", Prompt: prompt, ResumeSchema: schema, Checkpoint: checkpoint}},
		{Kind: toolloop.EventResume, Round: 1, Resume: &toolloop.Resume{ID: "approval-1", Input: json.RawMessage(`"approved"`)}},
	}

	for _, event := range events {
		t.Run(string(event.Kind), func(t *testing.T) {
			body, err := json.Marshal(event)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			var decoded toolloop.Event
			if err := json.Unmarshal(body, &decoded); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if !reflect.DeepEqual(decoded, event) {
				t.Fatalf("round trip = %#v, want %#v", decoded, event)
			}
		})
	}
}

func TestEventValidationRejectsAmbiguousOrInvalidPayload(t *testing.T) {
	request := protocolRequest(t)
	call := &chat.ToolCall{ID: "call-1", Name: "lookup"}

	for _, test := range []struct {
		name  string
		event toolloop.Event
	}{
		{name: "unknown kind", event: toolloop.Event{Kind: "unknown", Round: 1, Request: request}},
		{name: "no payload", event: toolloop.Event{Kind: toolloop.EventModelRequest}},
		{name: "multiple payloads", event: toolloop.Event{Kind: toolloop.EventModelRequest, Round: 1, Request: request, ToolCall: call}},
		{name: "wrong payload", event: toolloop.Event{Kind: toolloop.EventModelRequest, Round: 1, ToolCall: call}},
		{name: "wrong response payload", event: toolloop.Event{Kind: toolloop.EventModelResponse, Round: 1, Request: request}},
		{name: "wrong call payload", event: toolloop.Event{Kind: toolloop.EventToolCall, Round: 1, Request: request}},
		{name: "wrong result payload", event: toolloop.Event{Kind: toolloop.EventToolResult, Round: 1, Request: request}},
		{name: "wrong pause payload", event: toolloop.Event{Kind: toolloop.EventPause, Round: 1, Request: request}},
		{name: "wrong resume payload", event: toolloop.Event{Kind: toolloop.EventResume, Round: 1, Request: request}},
		{name: "invalid nested request", event: toolloop.Event{Kind: toolloop.EventModelRequest, Round: 1, Request: &chat.Request{}}},
		{name: "invalid nested response", event: toolloop.Event{Kind: toolloop.EventModelResponse, Round: 1, Response: &chat.Response{Choices: []chat.Choice{{Index: -1}}}}},
		{name: "invalid tool call", event: toolloop.Event{Kind: toolloop.EventToolCall, Round: 1, ToolCall: &chat.ToolCall{}}},
		{name: "invalid tool result", event: toolloop.Event{Kind: toolloop.EventToolResult, Round: 1, ToolResult: &chat.ToolResult{}}},
		{name: "invalid pause", event: toolloop.Event{Kind: toolloop.EventPause, Round: 1, Pause: &toolloop.Pause{ID: "pause"}}},
		{name: "invalid final kind", event: toolloop.Event{Kind: toolloop.EventModelRequest, Round: 1, Final: true, Request: request}},
		{name: "invalid resume", event: toolloop.Event{Kind: toolloop.EventResume, Round: 1, Resume: &toolloop.Resume{}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.event.Validate(); !errors.Is(err, toolloop.ErrInvalidEvent) {
				t.Fatalf("Validate error = %v", err)
			}
			if _, err := json.Marshal(test.event); !errors.Is(err, toolloop.ErrInvalidEvent) {
				t.Fatalf("Marshal error = %v", err)
			}
		})
	}
}

func TestEventUnmarshalDoesNotMutateOnFailure(t *testing.T) {
	original := toolloop.Event{
		Kind:  toolloop.EventPause,
		Round: 1,
		Pause: &toolloop.Pause{ID: "approval-1", Reason: "wait", Prompt: json.RawMessage(`"approve?"`), ResumeSchema: json.RawMessage(`{"type":"string"}`), Checkpoint: protocolCheckpoint(t)},
	}
	event := original
	if err := json.Unmarshal([]byte(`{"kind":"resume","resume":{}}`), &event); !errors.Is(err, toolloop.ErrInvalidEvent) {
		t.Fatalf("Unmarshal error = %v", err)
	}
	if !reflect.DeepEqual(event, original) {
		t.Fatalf("failed Unmarshal mutated event to %#v", event)
	}
	if err := json.Unmarshal([]byte(`{`), &event); err == nil {
		t.Fatalf("malformed Unmarshal error = %v", err)
	}
	if err := json.Unmarshal([]byte(`{"kind":"resume","round":1,"resume":{"id":"approval-1","input":true},"future":true}`), &event); !errors.Is(err, toolloop.ErrInvalidEvent) {
		t.Fatalf("unknown field error = %v", err)
	}
	var nilEvent *toolloop.Event
	if err := nilEvent.UnmarshalJSON([]byte(`{}`)); !errors.Is(err, toolloop.ErrInvalidEvent) {
		t.Fatalf("nil receiver error = %v", err)
	}
}

func TestCheckpointRejectsUnstablePauseID(t *testing.T) {
	checkpoint := protocolCheckpoint(t)
	checkpoint.ID = " approval-1 "
	if err := checkpoint.Validate(); !errors.Is(err, interaction.ErrInvalidID) {
		t.Fatalf("Validate error = %v", err)
	}
}

func TestChatResponseDoesNotContainToolLoopState(t *testing.T) {
	typeOfResponse := reflect.TypeFor[chat.Response]()
	for _, field := range []string{"ToolResult", "Pause", "Resume", "Event"} {
		if _, exists := typeOfResponse.FieldByName(field); exists {
			t.Fatalf("chat.Response unexpectedly contains runtime field %q", field)
		}
	}
}

func messagePointer(message chat.Message) *chat.Message { return &message }

func protocolCheckpoint(t *testing.T) *toolloop.Checkpoint {
	t.Helper()
	registry := protocolRegistryWithCall(t, func(context.Context, string) (string, error) {
		return "", &toolloop.PauseError{
			ID: "approval-1", Reason: "wait",
			Prompt: json.RawMessage(`"approve?"`), ResumeSchema: json.RawMessage(`{"type":"string"}`),
		}
	})
	model := &scriptedModel{call: func(int, *chat.Request) (*chat.Response, error) {
		return runnerToolResponse(chat.ToolCall{ID: "call-1", Name: "lookup", Arguments: `{}`}), nil
	}}
	runner := newRunner(t, model, toolloop.Config{})
	events, err := collectRunnerEvents(runner.Run(context.Background(), newRunnerRequest(t, registry), registry))
	if err != nil {
		t.Fatalf("create checkpoint: %v", err)
	}
	last := events[len(events)-1]
	if last.Pause == nil || last.Pause.Checkpoint == nil {
		t.Fatalf("checkpoint event = %#v", last)
	}
	return last.Pause.Checkpoint
}
