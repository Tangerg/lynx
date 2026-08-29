package chat_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/Tangerg/scope/core/chat"
)

func TestToolCallValidate(t *testing.T) {
	tests := []struct {
		name string
		call chat.ToolCall
		ok   bool
	}{
		{name: "valid", call: validToolCall(), ok: true},
		{name: "empty arguments", call: chat.ToolCall{ID: "call", Name: "tool"}, ok: true},
		{name: "malformed arguments", call: chat.ToolCall{ID: "call", Name: "tool", Arguments: `{`}, ok: true},
		{name: "empty ID", call: chat.ToolCall{Name: "tool", Arguments: `{}`}},
		{name: "empty name", call: chat.ToolCall{ID: "call", Arguments: `{}`}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call.Validate()
			if tt.ok && err != nil {
				t.Fatalf("Validate: %v", err)
			}
			if !tt.ok && !errors.Is(err, chat.ErrInvalidToolCall) {
				t.Fatalf("Validate error = %v, want ErrInvalidToolCall", err)
			}
		})
	}
}

func TestToolResultValidate(t *testing.T) {
	if err := validToolResult().Validate(); err != nil {
		t.Fatalf("valid result: %v", err)
	}
	if err := (chat.ToolResult{Name: "tool"}).Validate(); !errors.Is(err, chat.ErrInvalidToolResult) {
		t.Fatalf("empty ID error = %v", err)
	}
	if err := (chat.ToolResult{ID: "call"}).Validate(); !errors.Is(err, chat.ErrInvalidToolResult) {
		t.Fatalf("empty name error = %v", err)
	}
	if err := (chat.ToolResult{ID: "call", Name: "tool"}).Validate(); err != nil {
		t.Fatalf("empty result must be valid: %v", err)
	}
}

func TestToolOutputPreservesStructuredAndMediaContent(t *testing.T) {
	structured, err := chat.NewJSONToolOutput(json.RawMessage(`{"ok":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if text, ok := structured.Text(); !ok || text != `{"ok":true}` {
		t.Fatalf("structured Text = %q, %v", text, ok)
	}

	mediaOutput := chat.ToolOutput{Content: []chat.Part{chat.NewMediaPart(mustImage(t))}}
	if err := mediaOutput.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, ok := mediaOutput.Text(); ok {
		t.Fatal("media output reported a lossless text projection")
	}

	invalid := chat.ToolOutput{Content: []chat.Part{chat.NewToolCallPart(validToolCall())}}
	if err := invalid.Validate(); err == nil {
		t.Fatal("nested tool call was accepted as Tool output content")
	}
	if _, err := chat.NewJSONToolOutput(json.RawMessage(`{`)); err == nil {
		t.Fatal("malformed structured details were accepted")
	}
}

func TestToolCallDeltaValidate(t *testing.T) {
	if err := (chat.ToolCallDelta{ID: "call", Name: "tool", Arguments: "{"}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (chat.ToolCallDelta{Name: "tool"}).Validate(); !errors.Is(err, chat.ErrInvalidToolCall) {
		t.Fatalf("error = %v, want ErrInvalidToolCall", err)
	}
}
