package server

import (
	"context"
	"strconv"
	"strings"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/lyra/rpc/protocol"
)

// ListMessages returns a session's persisted history as wire messages.
//
// The backing store (chat-memory JSONL) is append-only, so a message's
// 1-based position is a stable id ("m1", "m2", …) — that's what
// messages.edit / sessions.fork address. Cursor pagination isn't wired
// yet (mirrors sessions.list): a cursor returns ErrNotImplemented, and
// without one the whole history comes back as a single page.
func (i *Server) ListMessages(ctx context.Context, in protocol.ListMessagesRequest) (*protocol.Page[protocol.Message], error) {
	if in.SessionID == "" {
		return nil, protocol.ErrNotImplemented
	}
	if in.Cursor != "" {
		return nil, protocol.ErrNotImplemented
	}
	history, err := i.rt.ReadHistory(ctx, in.SessionID)
	if err != nil {
		return nil, err
	}
	items := wireMessages(in.SessionID, history)

	limit := in.Limit
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	hasMore := false
	if limit < len(items) {
		items = items[:limit]
		hasMore = true
	}
	return &protocol.Page[protocol.Message]{Items: items, HasMore: hasMore}, nil
}

// EditMessage — depends on checkpoints / fork semantics that aren't
// wired through the engine yet.
func (i *Server) EditMessage(_ context.Context, _ protocol.EditMessageRequest) (*protocol.EditMessageResponse, error) {
	return nil, notImpl("messages.edit")
}

// historyPrefix returns the slice of history up to and including the
// chat.Message that owns wire message atMessageID ("m<n>"). Because a
// tool message expands to several wire messages, the boundary is taken
// at the owning chat.Message — forking at the first of a tool message's
// returns includes that whole message. An empty / unparseable id (or one
// past the end) yields the whole history, i.e. "fork at the tip".
func historyPrefix(history []chat.Message, atMessageID string) []chat.Message {
	n, ok := parseMessageSeq(atMessageID)
	if !ok {
		return history
	}
	wire := 0
	for i, msg := range history {
		wire += wireMessageCount(msg)
		if wire >= n {
			return history[:i+1]
		}
	}
	return history
}

// parseMessageSeq decodes a wire message id ("m" + 1-based index) back
// to its index. Returns ok=false for empty / malformed ids.
func parseMessageSeq(id string) (int, bool) {
	rest, ok := strings.CutPrefix(id, "m")
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(rest)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// wireMessageCount is the number of wire messages one chat.Message
// expands to — 1 for everything except a tool message, which yields one
// per non-nil return. Keeps [historyPrefix] aligned with the id scheme
// in [wireMessages].
func wireMessageCount(msg chat.Message) int {
	if tm, ok := msg.(*chat.ToolMessage); ok {
		c := 0
		for _, r := range tm.ToolReturns {
			if r != nil {
				c++
			}
		}
		return c
	}
	return 1
}

// wireMessages converts persisted [chat.Message]s to wire messages,
// assigning stable 1-based sequence ids over the FLATTENED output. A
// chat.ToolMessage carrying N tool returns expands to N wire messages
// (role=tool, one per return) since the wire shape pairs one
// ToolCallID with one Content; everything else is 1:1. CreatedAt stays
// zero — the JSONL store doesn't record per-message timestamps.
func wireMessages(sessionID string, msgs []chat.Message) []protocol.Message {
	out := make([]protocol.Message, 0, len(msgs))
	next := func(role protocol.MessageRole) protocol.Message {
		return protocol.Message{
			ID:        "m" + strconv.Itoa(len(out)+1),
			SessionID: sessionID,
			Role:      role,
		}
	}
	for _, msg := range msgs {
		switch m := msg.(type) {
		case *chat.SystemMessage:
			wm := next(protocol.MessageRoleSystem)
			wm.Content = m.Text
			out = append(out, wm)
		case *chat.UserMessage:
			wm := next(protocol.MessageRoleUser)
			wm.Content = m.Text
			out = append(out, wm)
		case *chat.AssistantMessage:
			wm := next(protocol.MessageRoleAssistant)
			wm.Content = m.JoinedText()
			for _, call := range m.CollectToolCalls() {
				wm.ToolCalls = append(wm.ToolCalls, protocol.ToolCall{
					ID:        call.ID,
					Name:      call.Name,
					Arguments: call.Arguments,
				})
			}
			out = append(out, wm)
		case *chat.ToolMessage:
			for _, ret := range m.ToolReturns {
				if ret == nil {
					continue
				}
				wm := next(protocol.MessageRoleTool)
				wm.ToolCallID = ret.ID
				wm.Content = ret.Result
				out = append(out, wm)
			}
		}
	}
	return out
}
