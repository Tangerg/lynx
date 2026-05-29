package server

import (
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/lyra/rpc/protocol"
)

// TestWireMessages maps each chat.Message variant to its wire shape and
// checks the stable 1-based sequence ids — including that a tool
// message with two returns flattens into two wire messages (so the ids
// stay contiguous over the flattened output, not the source slice).
func TestWireMessages(t *testing.T) {
	toolMsg, err := chat.NewToolMessage([]*chat.ToolReturn{
		{ID: "c1", Name: "bash", Result: "ok"},
		{ID: "c2", Name: "grep", Result: "match"},
	})
	if err != nil {
		t.Fatalf("NewToolMessage: %v", err)
	}
	msgs := []chat.Message{
		chat.NewSystemMessage("you are lyra"),
		chat.NewUserMessage("find TODOs"),
		chat.NewAssistantMessage([]*chat.ToolCallPart{{ID: "c1", Name: "bash", Arguments: `{"cmd":"ls"}`}}),
		toolMsg,
	}

	got := wireMessages("sess-1", msgs)

	// 4 source messages, but the tool message's 2 returns flatten → 5.
	if len(got) != 5 {
		t.Fatalf("wire message count = %d, want 5", len(got))
	}
	want := []struct {
		id         string
		role       protocol.MessageRole
		content    string
		toolCallID string
		toolCalls  int
	}{
		{"m1", protocol.MessageRoleSystem, "you are lyra", "", 0},
		{"m2", protocol.MessageRoleUser, "find TODOs", "", 0},
		{"m3", protocol.MessageRoleAssistant, "", "", 1},
		{"m4", protocol.MessageRoleTool, "ok", "c1", 0},
		{"m5", protocol.MessageRoleTool, "match", "c2", 0},
	}
	for i, w := range want {
		g := got[i]
		if g.ID != w.id || g.Role != w.role || g.Content != w.content ||
			g.ToolCallID != w.toolCallID || len(g.ToolCalls) != w.toolCalls {
			t.Errorf("msg[%d] = %+v, want id=%s role=%s content=%q toolCallID=%s toolCalls=%d",
				i, g, w.id, w.role, w.content, w.toolCallID, w.toolCalls)
		}
		if g.SessionID != "sess-1" {
			t.Errorf("msg[%d] SessionID = %q, want sess-1", i, g.SessionID)
		}
	}
	// The assistant tool call is carried through verbatim.
	if tc := got[2].ToolCalls; len(tc) == 1 && (tc[0].ID != "c1" || tc[0].Name != "bash") {
		t.Errorf("assistant tool call = %+v", tc[0])
	}
}

// TestHistoryPrefix checks the fork-point slicing, including that a
// wire id landing inside a multi-return tool message takes the whole
// owning chat.Message, and that empty / out-of-range ids fork at the tip.
func TestHistoryPrefix(t *testing.T) {
	toolMsg, err := chat.NewToolMessage([]*chat.ToolReturn{
		{ID: "c1", Name: "bash", Result: "ok"},
		{ID: "c2", Name: "grep", Result: "match"},
	})
	if err != nil {
		t.Fatalf("NewToolMessage: %v", err)
	}
	// wire ids: m1 system, m2 user, m3 assistant, m4 tool-ret1, m5 tool-ret2
	history := []chat.Message{
		chat.NewSystemMessage("sys"),
		chat.NewUserMessage("hi"),
		chat.NewAssistantMessage([]*chat.ToolCallPart{{ID: "c1", Name: "bash", Arguments: "{}"}}),
		toolMsg,
	}
	cases := []struct {
		at      string
		wantLen int
	}{
		{"m1", 1},
		{"m2", 2},
		{"m3", 3},
		{"m4", 4},  // inside the tool message → whole tool message included
		{"m5", 4},  // second return of the same tool message → still 4
		{"", 4},    // empty id → fork at tip
		{"m99", 4}, // past the end → fork at tip
		{"bogus", 4},
	}
	for _, c := range cases {
		if got := historyPrefix(history, c.at); len(got) != c.wantLen {
			t.Errorf("historyPrefix(at=%q) len = %d, want %d", c.at, len(got), c.wantLen)
		}
	}
}
