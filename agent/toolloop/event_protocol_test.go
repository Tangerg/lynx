package toolloop_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

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

	events := []toolloop.Event{
		{Kind: toolloop.EventModelRequest, Request: request},
		{Kind: toolloop.EventModelResponse, Response: response},
		{Kind: toolloop.EventToolCall, ToolCall: call},
		{Kind: toolloop.EventToolResult, ToolResult: result},
		{Kind: toolloop.EventPause, Pause: &toolloop.Pause{ID: "approval-1", Reason: "approval required", Checkpoint: checkpoint}},
		{Kind: toolloop.EventResume, Resume: &toolloop.Resume{ID: "approval-1", Input: "approved"}},
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
		{name: "unknown kind", event: toolloop.Event{Kind: "unknown", Request: request}},
		{name: "no payload", event: toolloop.Event{Kind: toolloop.EventModelRequest}},
		{name: "multiple payloads", event: toolloop.Event{Kind: toolloop.EventModelRequest, Request: request, ToolCall: call}},
		{name: "wrong payload", event: toolloop.Event{Kind: toolloop.EventModelRequest, ToolCall: call}},
		{name: "wrong response payload", event: toolloop.Event{Kind: toolloop.EventModelResponse, Request: request}},
		{name: "wrong call payload", event: toolloop.Event{Kind: toolloop.EventToolCall, Request: request}},
		{name: "wrong result payload", event: toolloop.Event{Kind: toolloop.EventToolResult, Request: request}},
		{name: "wrong pause payload", event: toolloop.Event{Kind: toolloop.EventPause, Request: request}},
		{name: "wrong resume payload", event: toolloop.Event{Kind: toolloop.EventResume, Request: request}},
		{name: "invalid nested request", event: toolloop.Event{Kind: toolloop.EventModelRequest, Request: &chat.Request{}}},
		{name: "invalid nested response", event: toolloop.Event{Kind: toolloop.EventModelResponse, Response: &chat.Response{Choices: []chat.Choice{{Index: -1}}}}},
		{name: "invalid tool call", event: toolloop.Event{Kind: toolloop.EventToolCall, ToolCall: &chat.ToolCall{}}},
		{name: "invalid tool result", event: toolloop.Event{Kind: toolloop.EventToolResult, ToolResult: &chat.ToolResult{}}},
		{name: "invalid pause", event: toolloop.Event{Kind: toolloop.EventPause, Pause: &toolloop.Pause{ID: "pause"}}},
		{name: "invalid final kind", event: toolloop.Event{Kind: toolloop.EventModelRequest, Final: true, Request: request}},
		{name: "invalid resume", event: toolloop.Event{Kind: toolloop.EventResume, Resume: &toolloop.Resume{}}},
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
		Pause: &toolloop.Pause{ID: "approval-1", Reason: "wait", Checkpoint: protocolCheckpoint(t)},
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
	var nilEvent *toolloop.Event
	if err := nilEvent.UnmarshalJSON([]byte(`{}`)); !errors.Is(err, toolloop.ErrInvalidEvent) {
		t.Fatalf("nil receiver error = %v", err)
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
	request := protocolRequest(t)
	request.Tools = protocolRegistry(t).Definitions()
	return &toolloop.Checkpoint{
		ID:      "approval-1",
		Round:   1,
		Request: request,
		Response: &chat.Response{Choices: []chat.Choice{{
			Index: 0,
			Message: messagePointer(chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{
				ID: "call-1", Name: "lookup", Arguments: `{}`,
			}))),
		}}},
	}
}
