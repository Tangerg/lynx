package server

import (
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// TestTranslator_EditedArgsReusesProposalItem verifies that when the user edits
// a tool's args at approval, the re-fired call with different args still reuses
// the original proposal item id.
func TestTranslator_EditedArgsReusesProposalItem(t *testing.T) {
	rb := &resumeBinding{
		originRunID: "run_1",
		toolItems:   map[string]string{resumeKey("write", argsKey(map[string]any{"file_path": "a.txt"})): "item_orig"},
		byName:      map[string]string{"write": "item_orig"},
	}
	tr := newTranslator("ses_1", "run_1_cont", "run_1", nil, rb, "", "")

	id, runID := tr.reuseOrNextItemID("write", `{"file_path":"b.txt"}`)
	if id != "item_orig" || runID != "run_1" {
		t.Fatalf("edited-args re-fire = (%q,%q), want (item_orig, run_1) - original item must be reused", id, runID)
	}
	if id2, _ := tr.reuseOrNextItemID("write", `{"file_path":"c.txt"}`); id2 == "item_orig" {
		t.Errorf("fallback must be one-shot, got %q again", id2)
	}
}

// TestTranslator_NoUserMessageOnContinuation verifies a continuation run
// opens with run.started alone, with no synthetic user turn.
func TestTranslator_NoUserMessageOnContinuation(t *testing.T) {
	tr := newTranslator("ses_1", "run_1_cont", "run_1", nil, nil, "", "")
	out := tr.open()
	if len(out) != 1 || out[0].Type != protocol.StreamRunStarted {
		t.Fatalf("continuation open() = %+v, want run.started only", out)
	}
	if out[0].Run == nil || out[0].Run.ParentRunID != "run_1" {
		t.Fatalf("continuation run.started must carry parentRunId run_1: %+v", out[0].Run)
	}
}

// TestTranslator_ResumedToolReusesOriginalItemID verifies an approved tool
// re-firing in a continuation completes its original proposal item.
func TestTranslator_ResumedToolReusesOriginalItemID(t *testing.T) {
	const origItemID = "item_run_1_3"
	const args = `{"command":"ls"}`
	resume := &resumeBinding{
		originRunID: "run_1",
		toolItems:   map[string]string{resumeKey("shell", args): origItemID},
	}
	tr := newTranslator("ses_1", "run_1_cont", "run_1", nil, resume, "", "")

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
	if other.RunID != "run_1_cont" {
		t.Fatalf("fresh tool runId = %q, want continuation run_1_cont", other.RunID)
	}
}

// TestTranslator_ResumedQuestionCompletes verifies a question item left
// in-progress at interrupt gets its terminal item.completed in the continuation.
func TestTranslator_ResumedQuestionCompletes(t *testing.T) {
	const qItemID = "item_run_1_1"
	q := &protocol.Question{Prompt: "what next?", Fields: []protocol.QuestionField{{Name: "answer", Type: "text"}}}
	resume := &resumeBinding{
		originRunID: "run_1",
		questions:   []resumedQuestion{{itemID: qItemID, question: q}},
	}
	tr := newTranslator("ses_1", "run_1_cont", "run_1", nil, resume, "", "")

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
