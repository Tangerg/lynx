package chat_test

import (
	"encoding/json"
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
		if m.JoinedText() != "answer" {
			t.Fatalf("JoinedText = %q, want answer", m.JoinedText())
		}
		if m.HasToolCalls() {
			t.Fatal("no tool calls expected")
		}
		if m.Metadata == nil {
			t.Fatal("Metadata must be allocated, never nil")
		}
	})

	t.Run("tool calls slice", func(t *testing.T) {
		calls := []*chat.ToolCallPart{{ID: "1", Name: "f", Arguments: "{}"}}
		m := chat.NewAssistantMessage(calls)
		if !m.HasToolCalls() {
			t.Fatal("ToolCalls not threaded")
		}
	})

	t.Run("output parts", func(t *testing.T) {
		parts := []chat.OutputPart{
			&chat.TextPart{Text: "hello "},
			&chat.ToolCallPart{ID: "1", Name: "f", Arguments: "{}"},
			&chat.TextPart{Text: "world"},
		}
		m := chat.NewAssistantMessage(parts)
		if len(m.Parts) != 3 {
			t.Fatalf("Parts len = %d, want 3", len(m.Parts))
		}
		if m.JoinedText() != "hello world" {
			t.Fatalf("JoinedText = %q", m.JoinedText())
		}
	})

	t.Run("metadata", func(t *testing.T) {
		md := map[string]any{"k": 1}
		m := chat.NewAssistantMessage(md)
		if v, _ := m.Meta()["k"]; v != 1 {
			t.Fatalf("Meta()[k] = %v, want 1", v)
		}
	})

	t.Run("full params with reasoning", func(t *testing.T) {
		m := chat.NewAssistantMessage(chat.MessageParams{
			Parts: []chat.OutputPart{
				&chat.ReasoningPart{Text: "thinking..."},
				&chat.TextPart{Text: "t"},
			},
		})
		if m.JoinedReasoning() != "thinking..." {
			t.Fatalf("JoinedReasoning = %q", m.JoinedReasoning())
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
		Parts: []chat.OutputPart{
			&chat.TextPart{Text: "calling tool"},
			&chat.ToolCallPart{ID: "1", Name: "f", Arguments: "{}"},
		},
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

func TestMessage_JSONRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		msg  chat.Message
	}{
		{"system", chat.NewSystemMessage("you are concise")},
		{"user", chat.NewUserMessage("hi")},
		{"assistant", chat.NewAssistantMessage(chat.MessageParams{
			Parts: []chat.OutputPart{
				&chat.ReasoningPart{Text: "thinking out loud"},
				&chat.TextPart{Text: "answer"},
				&chat.ToolCallPart{ID: "c1", Name: "search", Arguments: `{"q":"x"}`},
			},
		})},
	}
	tool, _ := chat.NewToolMessage([]*chat.ToolReturn{{ID: "c1", Name: "search", Result: "ok"}})
	cases = append(cases, struct {
		name string
		msg  chat.Message
	}{"tool", tool})

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.msg)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if !strings.Contains(string(data), `"type":"`+string(tc.msg.Type())+`"`) {
				t.Fatalf("marshaled JSON missing type discriminator: %s", data)
			}
			got, err := chat.UnmarshalMessage(data)
			if err != nil {
				t.Fatalf("UnmarshalMessage: %v", err)
			}
			if got.Type() != tc.msg.Type() {
				t.Fatalf("Type = %q, want %q", got.Type(), tc.msg.Type())
			}
		})
	}
}

func TestRequest_JSONRoundTrip(t *testing.T) {
	tool, _ := chat.NewToolMessage([]*chat.ToolReturn{{ID: "c1", Name: "search", Result: "ok"}})
	req, err := chat.NewRequest([]chat.Message{
		chat.NewSystemMessage("be concise"),
		chat.NewUserMessage("hi"),
		chat.NewAssistantMessage(chat.MessageParams{Text: "hello"}),
		tool,
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got chat.Request
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(got.Messages) != 4 {
		t.Fatalf("Messages len = %d, want 4", len(got.Messages))
	}
	wantTypes := []chat.MessageType{
		chat.MessageTypeSystem,
		chat.MessageTypeUser,
		chat.MessageTypeAssistant,
		chat.MessageTypeTool,
	}
	for i, want := range wantTypes {
		if got.Messages[i].Type() != want {
			t.Fatalf("Messages[%d].Type() = %q, want %q", i, got.Messages[i].Type(), want)
		}
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
