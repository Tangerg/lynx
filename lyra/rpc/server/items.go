package server

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

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
		return i.listItemsFromHistory(ctx, store, in)
	}
	return i.listItemsFromMessages(ctx, in)
}

// listItemsFromHistory serves items.list from the durable Item store.
func (i *Server) listItemsFromHistory(ctx context.Context, store history.Store, in protocol.ListItemsRequest) (*protocol.ListItemsResponse, error) {
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
		i.reconcileLostRun(&r)
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

// reconcileLostRun heals a RunRef the durable history left at status:running
// when no live pump is driving it: such a run was lost to a restart/crash
// between run.started and its terminal run.finished. Nothing is advancing it
// and it isn't resumable (no interrupt was recorded — a parked run finishes
// with outcome:interrupt, which IS terminal in history), so without this the
// client renders a perpetual spinner. We present it as a terminal
// error(run_lost) (API.md §6.2 anti-dangling, applied to non-parked runs).
//
// Reconciliation is in-memory on the read path, not a write-back: it re-judges
// liveness from the run table on every items.list, so a run is never wrongly
// terminalized (the table entry is set before the first persist and cleared
// only after the terminal one — a genuinely live run is always present). No-op
// for already-terminal runs.
func (i *Server) reconcileLostRun(r *protocol.RunRef) {
	if r.Status != protocol.RunStatusRunning || i.isRunLive(r.ID) {
		return
	}
	r.Status = protocol.RunStatusFinished
	r.Outcome = &protocol.RunOutcome{
		Type:   protocol.OutcomeError,
		Result: &protocol.RunResult{Error: &protocol.ProblemData{Type: "run_lost", Detail: "run lost on restart"}},
	}
	if r.FinishedAt.IsZero() {
		r.FinishedAt = time.Now().UTC()
	}
}

// isRunLive reports whether a run is currently being pumped in this process.
func (i *Server) isRunLive(runID string) bool {
	i.runMu.Lock()
	defer i.runMu.Unlock()
	_, ok := i.runs[runID]
	return ok
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
	// toolCallID → where its item lives + the raw name/args, so the matching
	// ToolMessage can rebuild the full ToolInvocation from the output.
	type toolRef struct {
		idx        int
		name, args string
	}
	byCallID := map[string]toolRef{}
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
				byCallID[call.ID] = toolRef{idx: len(out), name: call.Name, args: call.Arguments}
				out = append(out, protocol.Item{
					ID:     nextID(),
					Status: protocol.ItemStatusCompleted,
					Type:   protocol.ItemTypeToolCall,
					Tool:   toolInvocation(call.Name, call.Arguments, ""),
				})
			}
		case *chat.ToolMessage:
			for _, ret := range m.ToolReturns {
				if ret == nil {
					continue
				}
				if r, ok := byCallID[ret.ID]; ok {
					// Rebuild the full invocation now the output is known
					// (search hits / exit code / generic result land here).
					out[r.idx].Tool = toolInvocation(r.name, r.args, ret.Result)
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
