package server

import (
	"strconv"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
)

// itemPair emits the started + completed pair for a single durable item whose
// content is fully known up front (no streaming deltas) — the shape shared by
// the opening userMessage, a mid-run steer, and a compaction divider. build
// stamps the same id / type / content under each status.
func itemPair(build func(protocol.ItemStatus) *protocol.Item) []protocol.StreamEvent {
	return []protocol.StreamEvent{
		{Type: protocol.StreamItemStarted, Item: build(protocol.ItemStatusRunning)},
		{Type: protocol.StreamItemCompleted, Item: build(protocol.ItemStatusCompleted)},
	}
}

// compaction surfaces a post-turn auto-compaction as a standalone compaction
// Item (item.started + item.completed, one durable id) so the client folds it
// into a "context compacted — N messages dropped" divider between turns
// (API.md §4.3). DroppedMessages is the net history reduction (before − after,
// clamped ≥0); the summary text stays server-side — it's already folded into
// the rewritten history. Emitted from drive() before TurnEnd, so the divider
// lands after this turn's content and ahead of run.finished.
func (t *translator) compaction(e turn.CompactBoundary) []protocol.StreamEvent {
	dropped := max(e.MessagesBefore-e.MessagesAfter, 0)
	id := t.nextItemID()
	now := time.Now().UTC()
	item := func(status protocol.ItemStatus) *protocol.Item {
		return &protocol.Item{
			ID:              id,
			RunID:           t.runID,
			Status:          status,
			Type:            protocol.ItemTypeCompaction,
			CreatedAt:       now,
			DroppedMessages: dropped,
		}
	}
	return itemPair(item)
}

// openUserMessage emits the run's opening user turn as a userMessage Item
// (item.started + item.completed) so the live stream carries it — the
// client renders the user bubble straight from the event flow and learns
// its durable item id (matching items.list on reload). Emitted once: it
// consumes t.userInput. Empty for continuation runs.
func (t *translator) openUserMessage() []protocol.StreamEvent {
	if len(t.userInput) == 0 {
		return nil
	}
	input := t.userInput
	t.userInput = nil
	id := userMessageItemID(t.runID)
	now := time.Now().UTC()
	item := func(status protocol.ItemStatus) *protocol.Item {
		return &protocol.Item{
			ID:        id,
			RunID:     t.runID,
			Status:    status,
			Type:      protocol.ItemTypeUserMessage,
			CreatedAt: now,
			Content:   input,
		}
	}
	return itemPair(item)
}

// steerMessage surfaces a mid-run steering turn as its own userMessage Item: a
// fresh id per steer (not the run's fixed opening id) so repeated steers don't
// collide, with any open assistant text / reasoning closed first so the user
// turn never nests inside one. The injected message is already in the model's
// context (the loop appended it after the latest tool result); this is its
// timeline + durable-transcript record, shaped exactly like the opening turn.
func (t *translator) steerMessage(e turn.SteerMessage) []protocol.StreamEvent {
	out := t.closeReasoning()
	out = append(out, t.closeText()...)
	id := t.nextItemID()
	now := time.Now().UTC()
	item := func(status protocol.ItemStatus) *protocol.Item {
		return &protocol.Item{
			ID:        id,
			RunID:     t.runID,
			Status:    status,
			Type:      protocol.ItemTypeUserMessage,
			CreatedAt: now,
			Content:   []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: e.Text}},
		}
	}
	return append(out, itemPair(item)...)
}

// todosSnapshot projects the model's task list onto a state.snapshot under the
// "todos" key (the frontend reads shared["todos"]). The list is replaced whole,
// so the id is positional. Maps the domain Item (Content/Status) to the wire
// TodoSnapshot (text/status); status strings already match the wire vocab.
func (t *translator) todosSnapshot(e turn.TodosUpdated) []protocol.StreamEvent {
	todos := make([]protocol.TodoSnapshot, len(e.Todos))
	for i, it := range e.Todos {
		todos[i] = protocol.TodoSnapshot{
			ID:            strconv.Itoa(i),
			Text:          it.Content,
			Status:        string(it.Status),
			BlockedReason: it.BlockedReason,
			NextAction:    it.NextAction,
		}
	}
	return []protocol.StreamEvent{{
		Type:  protocol.StreamStateSnapshot,
		State: map[string]any{"todos": todos},
	}}
}
