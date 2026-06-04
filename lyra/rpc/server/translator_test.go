package server

import (
	"testing"

	"github.com/Tangerg/lynx/lyra/internal/service/chat"
	"github.com/Tangerg/lynx/lyra/rpc/protocol"
)

// TestTranslator_OpensUserMessageOnRootRun verifies a root run streams the
// user's input as the opening userMessage Item (item.started + item.completed)
// right after run.started — the event source the live view renders from, with
// an id that matches items.list on reload.
func TestTranslator_OpensUserMessageOnRootRun(t *testing.T) {
	input := []protocol.ContentBlock{{Type: "text", Text: "hello"}}
	tr := newTranslator("ses_1", "run_1", "", input)

	out := tr.translate(chat.TurnStart{Model: "deepseek-v4-flash"})
	if len(out) != 3 {
		t.Fatalf("TurnStart on a root run: got %d events, want 3 (run.started + userMessage started/completed)", len(out))
	}
	if out[0].Type != protocol.StreamRunStarted {
		t.Fatalf("event[0] = %s, want run.started", out[0].Type)
	}

	started, completed := out[1], out[2]
	if started.Type != protocol.StreamItemStarted || completed.Type != protocol.StreamItemCompleted {
		t.Fatalf("userMessage events = (%s, %s), want (item.started, item.completed)", started.Type, completed.Type)
	}
	for _, se := range []protocol.StreamEvent{started, completed} {
		if se.Item == nil || se.Item.Type != protocol.ItemTypeUserMessage {
			t.Fatalf("userMessage item missing or wrong type: %+v", se.Item)
		}
		if se.Item.RunID != "run_1" {
			t.Fatalf("userMessage RunID = %q, want run_1", se.Item.RunID)
		}
		if len(se.Item.Content) != 1 || se.Item.Content[0].Text != "hello" {
			t.Fatalf("userMessage content = %+v, want the input block", se.Item.Content)
		}
	}
	if started.Item.ID != completed.Item.ID {
		t.Fatalf("item.started id %q != item.completed id %q — must be one Item", started.Item.ID, completed.Item.ID)
	}
	if started.Item.Status != protocol.ItemStatusInProgress || completed.Item.Status != protocol.ItemStatusCompleted {
		t.Fatalf("statuses = (%s, %s), want (inProgress, completed)", started.Item.Status, completed.Item.Status)
	}

	// The opening user turn is emitted once — a second TurnStart (defensive)
	// must not re-emit it.
	if again := tr.translate(chat.TurnStart{}); len(again) != 1 || again[0].Type != protocol.StreamRunStarted {
		t.Fatalf("second TurnStart re-emitted the user message: %+v", again)
	}
}

// TestTranslator_NoUserMessageOnContinuation verifies a continuation run
// (runs.resume, nil input) emits run.started alone — no synthetic user turn.
func TestTranslator_NoUserMessageOnContinuation(t *testing.T) {
	tr := newTranslator("ses_1", "run_1_cont", "run_1", nil)
	out := tr.translate(chat.TurnStart{})
	if len(out) != 1 || out[0].Type != protocol.StreamRunStarted {
		t.Fatalf("continuation TurnStart = %+v, want run.started only", out)
	}
}
