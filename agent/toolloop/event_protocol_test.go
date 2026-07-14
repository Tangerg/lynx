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

	events := []toolloop.Event{
		{Kind: toolloop.EventModelRequest, Request: request},
		{Kind: toolloop.EventModelResponse, Response: response},
		{Kind: toolloop.EventToolCall, ToolCall: call},
		{Kind: toolloop.EventToolResult, ToolResult: result},
		{Kind: toolloop.EventPause, Pause: &toolloop.Pause{ID: "approval-1", Reason: "approval required"}},
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
		{name: "invalid nested request", event: toolloop.Event{Kind: toolloop.EventModelRequest, Request: &chat.Request{}}},
		{name: "invalid tool call", event: toolloop.Event{Kind: toolloop.EventToolCall, ToolCall: &chat.ToolCall{}}},
		{name: "invalid pause", event: toolloop.Event{Kind: toolloop.EventPause, Pause: &toolloop.Pause{ID: "pause"}}},
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
		Pause: &toolloop.Pause{ID: "pause-1", Reason: "wait"},
	}
	event := original
	if err := json.Unmarshal([]byte(`{"kind":"resume","resume":{}}`), &event); !errors.Is(err, toolloop.ErrInvalidEvent) {
		t.Fatalf("Unmarshal error = %v", err)
	}
	if !reflect.DeepEqual(event, original) {
		t.Fatalf("failed Unmarshal mutated event to %#v", event)
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
