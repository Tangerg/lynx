package server

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/lyra/internal/service/history"
	"github.com/Tangerg/lynx/lyra/rpc/protocol"
)

// ListItems returns a session's persisted history as durable Items
// (API.md §7.4). History = the completed Item sequence; there is no
// separate Message type. Cursor pagination isn't wired yet — without a
// cursor the whole history comes back as one page; a cursor returns
// capability_not_negotiated.
//
// The authoritative source is the durable Item-history store: the exact
// Items the runtime streamed (same ids, runId, text, createdAt) plus the
// RunRefs needed to rebuild the run tree (§10.3). When no history store
// is configured it falls back to reconstructing items from chat-memory
// messages — a flat list with no runId/run tree.
func (i *Server) ListItems(ctx context.Context, in protocol.ListItemsRequest) (*protocol.ListItemsResponse, error) {
	if in.Cursor != "" {
		return nil, notImpl("items.list (cursor)")
	}
	if store := i.rt.History(); store != nil {
		return listItemsFromHistory(ctx, store, in)
	}
	return i.listItemsFromMessages(ctx, in)
}

// listItemsFromHistory serves items.list from the durable Item store.
func listItemsFromHistory(ctx context.Context, store history.Store, in protocol.ListItemsRequest) (*protocol.ListItemsResponse, error) {
	hItems, hRuns, err := store.List(ctx, in.SessionID)
	if err != nil {
		return nil, err
	}
	items := make([]protocol.Item, 0, len(hItems))
	for _, hi := range hItems {
		var it protocol.Item
		if err := json.Unmarshal(hi.Blob, &it); err != nil {
			continue // skip a corrupt row rather than failing the whole list
		}
		items = append(items, it)
	}
	runs := make([]protocol.RunRef, 0, len(hRuns))
	for _, hr := range hRuns {
		var r protocol.RunRef
		if err := json.Unmarshal(hr.Blob, &r); err != nil {
			continue
		}
		runs = append(runs, r)
	}

	limit := in.Limit
	if limit <= 0 || limit > 200 {
		limit = 200
	}
	if limit < len(items) {
		items = items[:limit]
	}
	return &protocol.ListItemsResponse{Items: items, Runs: runs}, nil
}

// listItemsFromMessages is the fallback when no Item-history store is
// configured: reconstruct items from chat-memory messages. No runId / no
// run tree (Runs is empty) — clients render a flat item list.
func (i *Server) listItemsFromMessages(ctx context.Context, in protocol.ListItemsRequest) (*protocol.ListItemsResponse, error) {
	msgs, err := i.rt.ReadHistory(ctx, in.SessionID)
	if err != nil {
		return nil, err
	}
	items := historyToItems(in.SessionID, msgs)

	limit := in.Limit
	if limit <= 0 || limit > 200 {
		limit = 200
	}
	if limit < len(items) {
		items = items[:limit]
	}
	return &protocol.ListItemsResponse{Items: items, Runs: []protocol.RunRef{}}, nil
}

// EditItem — editing an item starts a continuation run (checkpoint
// semantics), which the engine doesn't support yet. Gated off
// (features.checkpoints).
func (i *Server) EditItem(_ context.Context, _ protocol.EditItemRequest) (*protocol.EditItemResponse, error) {
	return nil, notImpl("items.edit")
}

// historyToItems converts persisted chat.Messages into wire Items,
// assigning stable 1-based ids ("item_<sessionId>_<n>"). Tool returns
// (a trailing ToolMessage) are folded back into their originating
// toolCall Item's Output by matching ToolCallID. System messages are
// dropped — the system prompt is not part of the Item history.
func historyToItems(sessionID string, msgs []chat.Message) []protocol.Item {
	out := make([]protocol.Item, 0, len(msgs))
	byCallID := map[string]int{} // toolCallID → index into out
	seq := 0
	nextID := func() string {
		seq++
		return protocol.IDPrefixItem + sessionID + "_" + strconv.Itoa(seq)
	}

	for _, msg := range msgs {
		switch m := msg.(type) {
		case *chat.UserMessage:
			out = append(out, protocol.Item{
				ID:      nextID(),
				Status:  protocol.ItemStatusCompleted,
				Type:    protocol.ItemTypeUserMessage,
				Content: []protocol.ContentBlock{{Type: "text", Text: m.Text}},
			})
		case *chat.AssistantMessage:
			if text := m.JoinedText(); text != "" {
				out = append(out, protocol.Item{
					ID:      nextID(),
					Status:  protocol.ItemStatusCompleted,
					Type:    protocol.ItemTypeAgentMessage,
					Content: []protocol.ContentBlock{{Type: "text", Text: text}},
				})
			}
			for _, call := range m.CollectToolCalls() {
				item := protocol.Item{
					ID:     nextID(),
					Status: protocol.ItemStatusCompleted,
					Type:   protocol.ItemTypeToolCall,
					Tool: &protocol.ToolInvocation{
						Kind:      toolKind(call.Name),
						Name:      call.Name,
						Arguments: parseArgs(call.Arguments),
					},
				}
				byCallID[call.ID] = len(out)
				out = append(out, item)
			}
		case *chat.ToolMessage:
			for _, ret := range m.ToolReturns {
				if ret == nil {
					continue
				}
				if idx, ok := byCallID[ret.ID]; ok {
					out[idx].Tool.Output = ret.Result
				}
			}
		}
	}
	return out
}

// parseArgs decodes a tool call's JSON-encoded arguments into a
// structured object for the completed-item Tool.Arguments; nil when
// empty or unparseable.
func parseArgs(raw string) map[string]any {
	if raw == "" {
		return nil
	}
	var m map[string]any
	if json.Unmarshal([]byte(raw), &m) != nil {
		return nil
	}
	return m
}
