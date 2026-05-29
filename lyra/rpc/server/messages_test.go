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
