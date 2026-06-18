package server

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

// TestTranslator_OpensUserMessageOnRootRun verifies a root run streams the
// user's input as the opening userMessage Item (item.started + item.completed)
// right after run.started — the event source the live view renders from, with
// an id that matches items.list on reload.
func TestTranslator_OpensUserMessageOnRootRun(t *testing.T) {
	input := []protocol.ContentBlock{{Type: "text", Text: "hello"}}
	tr := newTranslator("ses_1", "run_1", "", input, nil, "")

	out := tr.open()
	if len(out) != 3 {
		t.Fatalf("open() on a root run: got %d events, want 3 (run.started + userMessage started/completed)", len(out))
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
	if started.Item.Status != protocol.ItemStatusRunning || completed.Item.Status != protocol.ItemStatusCompleted {
		t.Fatalf("statuses = (%s, %s), want (running, completed)", started.Item.Status, completed.Item.Status)
	}

	// The opening user turn is emitted once — open() consumed userInput, so a
	// second open() yields run.started alone (defensive; pumpRun calls once).
	if again := tr.open(); len(again) != 1 || again[0].Type != protocol.StreamRunStarted {
		t.Fatalf("second open() re-emitted the user message: %+v", again)
	}
	// The chat-level TurnStart is a no-op (run.started comes from open()).
	if ts := tr.translate(turn.TurnStart{Model: "deepseek-v4-flash"}); ts != nil {
		t.Fatalf("chat TurnStart should be a no-op, got %+v", ts)
	}
}

// TestTranslator_RunStartedCarriesModel verifies the run.started RunRef
// surfaces the run's model so the frontend can label the run (RunRef.model).
func TestTranslator_RunStartedCarriesModel(t *testing.T) {
	tr := newTranslator("ses_1", "run_1", "", nil, nil, "claude-opus-4-8")
	out := tr.open()
	if len(out) == 0 || out[0].Type != protocol.StreamRunStarted || out[0].Run == nil {
		t.Fatalf("first event = %+v, want run.started with a RunRef", out)
	}
	if r := out[0].Run; r.Model != "claude-opus-4-8" {
		t.Errorf("RunRef model = %q, want claude-opus-4-8", r.Model)
	}
}

// TestTranslator_EditedArgsReusesProposalItem verifies that when the user edits
// a tool's args at approval, the re-fired call (with different args) still
// reuses the ORIGINAL proposal item id — so its card completes instead of being
// orphaned "in progress" while a duplicate fresh card runs.
func TestTranslator_EditedArgsReusesProposalItem(t *testing.T) {
	rb := &resumeBinding{
		originRunID: "run_1",
		toolItems:   map[string]string{resumeKey("write", argsKey(map[string]any{"file_path": "a.txt"})): "item_orig"},
		byName:      map[string]string{"write": "item_orig"},
	}
	tr := newTranslator("ses_1", "run_1_cont", "run_1", nil, rb, "")

	// Re-fire with EDITED args (different path): the exact (name,args) key misses,
	// the name-only fallback hits.
	id, runID := tr.reuseOrNextItemID("write", `{"file_path":"b.txt"}`)
	if id != "item_orig" || runID != "run_1" {
		t.Fatalf("edited-args re-fire = (%q,%q), want (item_orig, run_1) — original item must be reused", id, runID)
	}
	// One-shot: a second re-fire of the same name no longer matches (fresh id).
	if id2, _ := tr.reuseOrNextItemID("write", `{"file_path":"c.txt"}`); id2 == "item_orig" {
		t.Errorf("fallback must be one-shot, got %q again", id2)
	}
}

// TestTranslator_NoUserMessageOnContinuation verifies a continuation run
// (runs.resume, nil input) opens with run.started alone — no synthetic user
// turn, and no chat TurnStart needed (continuations emit none).
func TestTranslator_NoUserMessageOnContinuation(t *testing.T) {
	tr := newTranslator("ses_1", "run_1_cont", "run_1", nil, nil, "")
	out := tr.open()
	if len(out) != 1 || out[0].Type != protocol.StreamRunStarted {
		t.Fatalf("continuation open() = %+v, want run.started only", out)
	}
	if out[0].Run == nil || out[0].Run.ParentRunID != "run_1" {
		t.Fatalf("continuation run.started must carry parentRunId run_1: %+v", out[0].Run)
	}
}

// TestTranslator_ResumedToolReusesOriginalItemID is the B2 regression: an
// approved tool re-firing in a continuation must complete its ORIGINAL
// proposal item (same id + origin runId), not a fresh item — so the proposal
// item gets its mandatory terminal item.completed and no duplicate card
// appears (API.md §5.2 / §6, itemId is the cross-interrupt correlation key).
func TestTranslator_ResumedToolReusesOriginalItemID(t *testing.T) {
	const origItemID = "item_run_1_3"
	const args = `{"command":"ls"}`
	resume := &resumeBinding{
		originRunID: "run_1",
		toolItems:   map[string]string{resumeKey("bash", args): origItemID},
	}
	tr := newTranslator("ses_1", "run_1_cont", "run_1", nil, resume, "")

	itemStarted := func(events []protocol.StreamEvent) *protocol.Item {
		for _, se := range events {
			if se.Type == protocol.StreamItemStarted {
				return se.Item
			}
		}
		return nil
	}

	start := itemStarted(tr.translate(turn.ToolCallStart{CallID: "call_x", ToolName: "bash", Arguments: args}))
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

	// A non-matching call (different args) gets a fresh id under the
	// continuation run — the binding is consumed once, no false reuse.
	other := itemStarted(tr.translate(turn.ToolCallStart{CallID: "call_y", ToolName: "bash", Arguments: `{"command":"pwd"}`}))
	if other == nil || other.ID == origItemID {
		t.Fatalf("non-matching tool reused the original id: %+v", other)
	}
	if other.RunID != "run_1_cont" {
		t.Fatalf("fresh tool runId = %q, want continuation run_1_cont", other.RunID)
	}
}

// TestTranslator_DeniedToolIsDistinct verifies a denied tool ends as a
// distinct terminal (incomplete + denied_by_user error), not a green success
// — so the UI can render "denied" rather than ✓.
func TestTranslator_DeniedToolIsDistinct(t *testing.T) {
	tr := newTranslator("ses_1", "run_1", "", nil, nil, "")
	tr.translate(turn.ToolCallStart{CallID: "c1", ToolName: "bash", Arguments: `{"command":"rm -rf /"}`})
	end := tr.translate(turn.ToolCallEnd{CallID: "c1", Output: "tool call denied by user", Denied: true})
	if len(end) != 1 || end[0].Item == nil {
		t.Fatalf("toolEnd = %+v, want one item.completed", end)
	}
	it := end[0].Item
	if it.Status != protocol.ItemStatusIncomplete {
		t.Fatalf("denied tool status = %s, want incomplete (not green completed)", it.Status)
	}
	if it.Error == nil || it.Error.Type != "denied_by_user" {
		t.Fatalf("denied tool error = %+v, want type denied_by_user", it.Error)
	}
}

// TestTranslator_ResumedQuestionCompletes verifies a question item (ask_user /
// exit_plan_mode) left inProgress at interrupt gets its terminal item.completed
// in the continuation (right after run.started) — it's resolved by the resume
// answer, not a re-fired event, so without this the proposal card stays
// "LIVE" forever (API.md §5.2). Same id + origin runId, content preserved.
func TestTranslator_ResumedQuestionCompletes(t *testing.T) {
	const qItemID = "item_run_1_1"
	q := &protocol.Question{Prompt: "what next?", Fields: []protocol.QuestionField{{Name: "answer", Type: "text"}}}
	resume := &resumeBinding{
		originRunID: "run_1",
		questions:   []resumedQuestion{{itemID: qItemID, question: q}},
	}
	tr := newTranslator("ses_1", "run_1_cont", "run_1", nil, resume, "")

	out := tr.open()
	// run.started first, then the question's terminal completion.
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

	// Emitted once — a second open() yields run.started alone.
	if again := tr.open(); len(again) != 1 {
		t.Fatalf("second open() re-emitted the question completion: %+v", again)
	}
}

// TestTranslator_CommandOutputOnCompleted is the §5.2 regression: a completed
// command tool MUST carry the authoritative output (merged stdout+stderr) on
// the item's tool.result.output — not only as a toolOutput delta preview.
// Without it, history replay / reconnect (no deltas) render "(no output)"
// even though the model saw the output (API.md §4.4.2 / §5.2). The domain-
// neutral envelope carries name + arguments + a best-effort JSON result.
func TestTranslator_CommandOutputOnCompleted(t *testing.T) {
	tr := newTranslator("ses_1", "run_1", "", nil, nil, "")
	tr.translate(turn.ToolCallStart{CallID: "c1", ToolName: "bash", Arguments: `{"command":"echo hi"}`})
	out := tr.translate(turn.ToolCallEnd{
		CallID: "c1",
		Output: `{"stdout":"hi\n","stderr":"oops","exit_code":0,"duration":"5ms"}`,
	})

	var completed *protocol.Item
	for _, se := range out {
		if se.Type == protocol.StreamItemCompleted {
			completed = se.Item
		}
	}
	if completed == nil || completed.Tool == nil {
		t.Fatalf("no completed toolCall item: %+v", out)
	}
	if completed.Tool.Name != "bash" {
		t.Fatalf("tool name = %q, want bash (name is the sole identity, §4.4)", completed.Tool.Name)
	}
	res, ok := completed.Tool.Result.(commandResult)
	if !ok {
		t.Fatalf("result = %T, want commandResult {exitCode,output,…} (§4.4.2)", completed.Tool.Result)
	}
	if got := res.Output; got != "hi\n\noops" {
		t.Fatalf("result.output = %q, want merged stdout+stderr %q", got, "hi\n\noops")
	}

	// A command with no output still carries result.output:"" (present, not
	// omitted) so the client renders an empty terminal rather than falling back
	// to a stale preview / "(no output)".
	tr2 := newTranslator("ses_1", "run_2", "", nil, nil, "")
	tr2.translate(turn.ToolCallStart{CallID: "c2", ToolName: "bash", Arguments: `{"command":"true"}`})
	out2 := tr2.translate(turn.ToolCallEnd{CallID: "c2", Output: `{"stdout":"","stderr":"","exit_code":0,"duration":"1ms"}`})
	for _, se := range out2 {
		if se.Type == protocol.StreamItemCompleted {
			res, ok := se.Item.Tool.Result.(commandResult)
			if !ok || res.Output != "" {
				t.Fatalf("empty-output command: result = %#v, want commandResult with output \"\"", se.Item.Tool.Result)
			}
		}
	}
}

// TestClassifyRunError buckets provider/transient failures into provider_error
// (with a clean, non-leaking detail) and leaves the rest internal_error — so
// the client can react (back off / re-key) instead of opaque-retrying, and the
// raw message (URL / Go call path) never reaches the wire.
func TestClassifyRunError(t *testing.T) {
	cases := []struct {
		name, msg, wantType string
		leakFragment        string // must NOT appear in the wire detail
	}{
		{"rate limit", `engine: run chat: POST "https://api.deepseek.com/v1": 429 Too Many Requests {"x"}`, "provider_error", "deepseek.com"},
		{"auth", `POST "https://api.deepseek.com": 401 Unauthorized`, "provider_error", "api.deepseek.com"},
		{"provider 5xx", `POST "https://api.x": 503 Service Unavailable`, "provider_error", "api.x"},
		{"timeout", `Post "https://api.x": context deadline exceeded`, "provider_error", "api.x"},
		{"bad request", `POST "https://api.x": 400 Bad Request invalid_request_error`, "provider_error", "api.x"},
		{"genuine internal", `engine: deploy chat agent: blackboard nil pointer`, "internal_error", ""},
		{"empty", ``, "internal_error", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tr := newTranslator("", "", "", nil, nil, "")
			got := tr.classifyRunError(c.msg)
			if got.Type != c.wantType {
				t.Fatalf("type = %q, want %q (msg=%q)", got.Type, c.wantType, c.msg)
			}
			if got.Detail == "" {
				t.Fatalf("detail must not be empty")
			}
			if c.leakFragment != "" && strings.Contains(got.Detail, c.leakFragment) {
				t.Fatalf("detail leaked %q: %q", c.leakFragment, got.Detail)
			}
		})
	}
}
