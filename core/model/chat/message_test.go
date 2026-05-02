package chat_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
)

func TestMessageType_Predicates(t *testing.T) {
	cases := []struct {
		name string
		t    chat.MessageType
		fn   func() bool
		want bool
	}{
		{"system", chat.MessageTypeSystem, chat.MessageTypeSystem.IsSystem, true},
		{"user not system", chat.MessageTypeUser, chat.MessageTypeUser.IsSystem, false},
		{"user", chat.MessageTypeUser, chat.MessageTypeUser.IsUser, true},
		{"assistant", chat.MessageTypeAssistant, chat.MessageTypeAssistant.IsAssistant, true},
		{"tool", chat.MessageTypeTool, chat.MessageTypeTool.IsTool, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.fn(); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNewMessage_Dispatch(t *testing.T) {
	cases := []struct {
		name    string
		params  chat.MessageParams
		wantTyp chat.MessageType
		wantErr bool
	}{
		{"system", chat.MessageParams{Type: chat.MessageTypeSystem, Text: "be brief"}, chat.MessageTypeSystem, false},
		{"user", chat.MessageParams{Type: chat.MessageTypeUser, Text: "hi"}, chat.MessageTypeUser, false},
		{"assistant", chat.MessageParams{Type: chat.MessageTypeAssistant, Text: "answer"}, chat.MessageTypeAssistant, false},
		{
			name:    "tool",
			params:  chat.MessageParams{Type: chat.MessageTypeTool, ToolReturns: []*chat.ToolReturn{{ID: "1", Name: "f", Result: "ok"}}},
			wantTyp: chat.MessageTypeTool,
		},
		{"unknown errors", chat.MessageParams{Type: "weird"}, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := chat.NewMessage(tc.params)
			if tc.wantErr {
				if err == nil {
					t.Fatal("want error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Type() != tc.wantTyp {
				t.Fatalf("Type = %s, want %s", got.Type(), tc.wantTyp)
			}
		})
	}
}

func TestNewAssistantMessage_Generic(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		m := chat.NewAssistantMessage("answer")
		if m.Text != "answer" || len(m.ToolCalls) != 0 || len(m.Media) != 0 {
			t.Fatalf("unexpected: %+v", m)
		}
		if m.Metadata == nil {
			t.Fatal("Metadata must be allocated, never nil")
		}
	})

	t.Run("tool calls", func(t *testing.T) {
		calls := []*chat.ToolCall{{ID: "1", Name: "f", Arguments: "{}"}}
		m := chat.NewAssistantMessage(calls)
		if !m.HasToolCalls() {
			t.Fatal("ToolCalls not threaded")
		}
	})

	t.Run("metadata", func(t *testing.T) {
		md := map[string]any{"k": 1}
		m := chat.NewAssistantMessage(md)
		if v, _ := m.Meta()["k"]; v != 1 {
			t.Fatalf("Meta()[k] = %v, want 1", v)
		}
	})

	t.Run("full params", func(t *testing.T) {
		m := chat.NewAssistantMessage(chat.MessageParams{
			Text:      "t",
			Reasoning: "thinking...",
		})
		if m.Reasoning != "thinking..." {
			t.Fatalf("Reasoning = %q", m.Reasoning)
		}
		if !m.HasReasoning() {
			t.Fatal("HasReasoning should be true")
		}
	})
}

func TestAssistantMessage_HasReasoning_NilSafe(t *testing.T) {
	var m *chat.AssistantMessage
	if m.HasReasoning() {
		t.Fatal("nil receiver should report false, never panic")
	}
}

func TestNewToolMessage_RequiresToolReturns(t *testing.T) {
	if _, err := chat.NewToolMessage([]*chat.ToolReturn{}); err == nil {
		t.Fatal("empty ToolReturns must error")
	}
	if _, err := chat.NewToolMessage(chat.MessageParams{}); err == nil {
		t.Fatal("MessageParams without ToolReturns must error")
	}
}

func TestFilterMessages_PanicsOnNilPredicate(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on nil predicate")
		}
	}()
	_ = chat.FilterMessages([]chat.Message{}, nil)
}

func TestFilterMessagesByMessageTypes(t *testing.T) {
	user := chat.NewUserMessage("u")
	sys := chat.NewSystemMessage("s")
	assistant := chat.NewAssistantMessage("a")

	got := chat.FilterMessagesByMessageTypes(
		[]chat.Message{user, sys, assistant},
		chat.MessageTypeUser, chat.MessageTypeAssistant,
	)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
}

func TestFilterMessagesByMessageTypes_NoTypesPassthrough(t *testing.T) {
	in := []chat.Message{chat.NewUserMessage("u")}
	got := chat.FilterMessagesByMessageTypes(in)
	if len(got) != 1 {
		t.Fatalf("expected passthrough, got len=%d", len(got))
	}
}

func TestMergeSystemMessages(t *testing.T) {
	a := chat.NewSystemMessage("first")
	b := chat.NewSystemMessage("second")

	merged := chat.MergeSystemMessages([]chat.Message{a, b})
	if merged == nil {
		t.Fatal("merge returned nil")
	}
	if !strings.Contains(merged.Text, "first") || !strings.Contains(merged.Text, "second") {
		t.Fatalf("merged Text = %q", merged.Text)
	}
}

func TestMergeUserMessages_ConcatenatesMediaAndText(t *testing.T) {
	a := chat.NewUserMessage("a")
	b := chat.NewUserMessage("b")

	merged := chat.MergeUserMessages([]chat.Message{a, b})
	if merged == nil {
		t.Fatal("merge returned nil")
	}
	if !strings.Contains(merged.Text, "a") || !strings.Contains(merged.Text, "b") {
		t.Fatalf("merged Text = %q", merged.Text)
	}
}

func TestMergeMessages_AssistantUnsupported(t *testing.T) {
	if _, err := chat.MergeMessages(nil, chat.MessageTypeAssistant); err == nil {
		t.Fatal("MergeMessages(assistant) must error")
	}
}

func TestMergeAdjacentSameTypeMessages(t *testing.T) {
	user1 := chat.NewUserMessage("u1")
	user2 := chat.NewUserMessage("u2")
	sys := chat.NewSystemMessage("s")

	got := chat.MergeAdjacentSameTypeMessages([]chat.Message{user1, user2, sys})
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (two adjacent users merge into one)", len(got))
	}
	if got[0].Type() != chat.MessageTypeUser {
		t.Fatalf("first type = %s, want user", got[0].Type())
	}
}

func TestMergeAdjacentSameTypeMessages_FiltersNil(t *testing.T) {
	user := chat.NewUserMessage("u")
	got := chat.MergeAdjacentSameTypeMessages([]chat.Message{nil, user, nil})
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
}

func TestMessageToString_AssistantWithToolCalls(t *testing.T) {
	m := chat.NewAssistantMessage(chat.MessageParams{
		Text:      "calling tool",
		ToolCalls: []*chat.ToolCall{{ID: "1", Name: "f", Arguments: "{}"}},
	})

	got := chat.MessageToString(m)

	if !strings.HasPrefix(got, "assistant: calling tool") {
		t.Fatalf("missing prefix in %q", got)
	}
	if !strings.Contains(got, `"name":"f"`) {
		t.Fatalf("ToolCalls JSON not embedded in %q", got)
	}
}

func TestMessageToString_Tool(t *testing.T) {
	m, err := chat.NewToolMessage([]*chat.ToolReturn{{ID: "1", Name: "f", Result: "ok"}})
	if err != nil {
		t.Fatal(err)
	}

	got := chat.MessageToString(m)
	if !strings.HasPrefix(got, "tool: ") {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(got, `"result":"ok"`) {
		t.Fatalf("ToolReturns JSON not embedded in %q", got)
	}
}

func TestMessagesToStrings(t *testing.T) {
	got := chat.MessagesToStrings([]chat.Message{
		chat.NewUserMessage("u"),
		chat.NewSystemMessage("s"),
	})
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if !strings.HasPrefix(got[0], "user: ") {
		t.Fatalf("got[0] = %q", got[0])
	}
	if !strings.HasPrefix(got[1], "system: ") {
		t.Fatalf("got[1] = %q", got[1])
	}
}
