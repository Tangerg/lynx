package chat_test

import (
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/chat"
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
	if err := (chat.ToolResult{ID: "call", Name: "tool", Result: ""}).Validate(); err != nil {
		t.Fatalf("empty result must be valid: %v", err)
	}
}
