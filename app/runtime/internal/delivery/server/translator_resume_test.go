package server

import (
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// A resume opens a new segment (seg_2) of the SAME run (run_1); reused proposal
// items keep their original ids (minted under the parked segment seg_1), fresh
// items derive from the continuation segment id — all under the one stable runId.

// TestTranslator_EditedArgsReusesProposalItem verifies that when the user edits
// a tool's args at approval, the re-fired call with different args still reuses
// the original proposal item id.
func TestTranslator_EditedArgsReusesProposalItem(t *testing.T) {
	rb := &resumeBinding{
		toolItems: map[string]string{resumeKey("write", argsKey(map[string]any{"file_path": "a.txt"})): "item_orig"},
		byName:    map[string]string{"write": "item_orig"},
	}
	tr := newTranslator("ses_1", "run_1", "seg_2", nil, rb, "", "")

	id := tr.reuseOrNextItemID("write", `{"file_path":"b.txt"}`)
	if id != "item_orig" {
		t.Fatalf("edited-args re-fire = %q, want item_orig - original item must be reused", id)
	}
	if id2 := tr.reuseOrNextItemID("write", `{"file_path":"c.txt"}`); id2 == "item_orig" {
		t.Errorf("fallback must be one-shot, got %q again", id2)
	}
}

// TestTranslator_NoUserMessageOnContinuation verifies a continuation segment
// opens with run.started alone (carrying the stable runId), no synthetic user turn.
func TestTranslator_NoUserMessageOnContinuation(t *testing.T) {
	tr := newTranslator("ses_1", "run_1", "seg_2", nil, nil, "", "")
	out := tr.open()
	if len(out) != 1 || out[0].Type != protocol.StreamRunStarted {
		t.Fatalf("continuation open() = %+v, want run.started only", out)
	}
	if out[0].Run == nil || out[0].Run.ID != "run_1" {
		t.Fatalf("continuation run.started must carry the stable runId run_1: %+v", out[0].Run)
	}
}

// TestTranslator_ResumedToolReusesOriginalItemID verifies an approved tool
// re-firing in a continuation completes its original proposal item, under the
// same stable runId — while a fresh tool gets a new (segment-derived) item id.
func TestTranslator_ResumedToolReusesOriginalItemID(t *testing.T) {
	const origItemID = "item_seg_1_3"
	const args = `{"command":"ls"}`
	resume := &resumeBinding{
		toolItems: map[string]string{resumeKey("shell", args): origItemID},
	}
	tr := newTranslator("ses_1", "run_1", "seg_2", nil, resume, "", "")

	itemStarted := func(events []protocol.StreamEvent) *protocol.Item {
		for _, se := range events {
			if se.Type == protocol.StreamItemStarted {
				return se.Item
			}
		}
		return nil
	}

	start := itemStarted(tr.translate(turn.ToolCallStart{CallID: "call_x", ToolName: "shell", Arguments: args}))
	if start == nil {
		t.Fatal("no item.started for the resumed tool")
	}
	if start.ID != origItemID || start.RunID != "run_1" {
		t.Fatalf("resumed tool item id/runId = %q/%q, want %q/run_1 (reuse the proposal item)", start.ID, start.RunID, origItemID)
	}

	end := tr.translate(turn.ToolCallEnd{CallID: "call_x", Output: "files"})
	if len(end) != 1 || end[0].Type != protocol.StreamItemCompleted {
		t.Fatalf("toolEnd = %+v, want one item.completed", end)
	}
	if end[0].Item.ID != origItemID || end[0].Item.RunID != "run_1" {
		t.Fatalf("completed id/runId = %q/%q, want %q/run_1 (closes the proposal)", end[0].Item.ID, end[0].Item.RunID, origItemID)
	}
	if end[0].Item.Status != protocol.ItemStatusCompleted {
		t.Fatalf("completed status = %s, want completed", end[0].Item.Status)
	}

	other := itemStarted(tr.translate(turn.ToolCallStart{CallID: "call_y", ToolName: "shell", Arguments: `{"command":"pwd"}`}))
	if other == nil || other.ID == origItemID {
		t.Fatalf("non-matching tool reused the original id: %+v", other)
	}
	if other.RunID != "run_1" {
		t.Fatalf("fresh tool runId = %q, want the stable run run_1", other.RunID)
	}
}

// TestTranslator_ResumedQuestionCompletes verifies a question item left
// in-progress at interrupt gets its terminal item.completed in the continuation.
func TestTranslator_ResumedQuestionCompletes(t *testing.T) {
	const qItemID = "item_seg_1_1"
	q := &protocol.Question{Prompt: "what next?", Fields: []protocol.QuestionField{{Name: "answer", Type: "text"}}}
	resume := &resumeBinding{
		questions: []resumedQuestion{{itemID: qItemID, question: q}},
	}
	tr := newTranslator("ses_1", "run_1", "seg_2", nil, resume, "", "")

	out := tr.open()
	if len(out) != 2 {
		t.Fatalf("continuation open() = %d events, want 2 (run.started + question item.completed)", len(out))
	}
	if out[0].Type != protocol.StreamRunStarted {
		t.Fatalf("event[0] = %s, want run.started", out[0].Type)
	}
	c := out[1]
	if c.Type != protocol.StreamItemCompleted || c.Item == nil {
		t.Fatalf("event[1] = %s, want item.completed", c.Type)
	}
	if c.Item.ID != qItemID || c.Item.RunID != "run_1" {
		t.Fatalf("question terminal id/runId = %q/%q, want %q/run_1", c.Item.ID, c.Item.RunID, qItemID)
	}
	if c.Item.Type != protocol.ItemTypeQuestion || c.Item.Status != protocol.ItemStatusCompleted {
		t.Fatalf("question terminal type/status = %s/%s, want question/completed", c.Item.Type, c.Item.Status)
	}
	if c.Item.Question == nil || c.Item.Question.Prompt != "what next?" {
		t.Fatalf("question terminal lost its content: %+v", c.Item.Question)
	}

	if again := tr.open(); len(again) != 1 {
		t.Fatalf("second open() re-emitted the question completion: %+v", again)
	}
}
