package server

import (
	"context"
	"errors"
	"testing"
	"time"

	appRuns "github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
	"github.com/Tangerg/lynx/core/chat"
)

// rollbackHarness wires a Server over a sqlite-backed stub: a real session
// service + history + interrupt store, plus the in-memory conversation map.
func rollbackHarness(t *testing.T) (*Server, *stubRuntime) {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	rt := &stubRuntime{
		sess:       sqlite.NewSessionStore(db),
		model:      "default-model",
		history:    map[string][]chat.Message{},
		hist:       sqlite.NewTranscriptStore(db),
		interrupts: sqlite.NewInterruptStore(db),
		muts:       sqlite.NewWorkspaceMutationStore(db),
	}
	return newTestServer(rt), rt
}

func putRun(t *testing.T, rt *stubRuntime, sessionID, runID string, atUnix int64, mark int) {
	t.Helper()
	outcome := execution.OutcomeCompleted
	if err := rt.hist.PutRun(t.Context(), transcript.Run{
		SessionID: sessionID, ID: runID, State: execution.Completed,
		Outcome: &outcome, Result: &transcript.RunResult{},
		CreatedAt: time.Unix(atUnix, 0).UTC(), FinishedAt: time.Unix(atUnix, 0).UTC(),
		UpdatedAt: time.Unix(atUnix, 0).UTC(), MessageMark: mark,
	}); err != nil {
		t.Fatalf("putRun %s: %v", runID, err)
	}
}

func putUserItem(t *testing.T, rt *stubRuntime, sessionID, runID, itemID, text string) {
	t.Helper()
	if err := rt.hist.AppendItem(t.Context(), transcript.Item{
		SessionID: sessionID, RunID: runID, ID: itemID, CreatedAt: time.Unix(1, 0).UTC(),
		Status: transcript.ItemCompleted, Kind: transcript.UserMessage,
		Content: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: text}},
	}); err != nil {
		t.Fatalf("putUserItem %s: %v", itemID, err)
	}
}

// TestRollbackSession_DropTail keeps the first turn and drops the second: the
// message log truncates to the kept run's watermark, the dropped run's history
// is deleted, and droppedRuns reports it with its opening user input.
func TestRollbackSession_DropTail(t *testing.T) {
	s, rt := rollbackHarness(t)
	ctx := context.Background()
	sess, _ := rt.sess.Create(ctx, "s", "/w")

	// Two completed turns: R1 left 3 messages, R2 left 6.
	rt.history[sess.ID] = []chat.Message{
		chat.NewUserMessage(chat.NewTextPart("u1")), chat.NewAssistantMessage(chat.NewTextPart("a1")), chat.NewUserMessage(chat.NewTextPart("u1b")),
		chat.NewUserMessage(chat.NewTextPart("u2")), chat.NewAssistantMessage(chat.NewTextPart("a2")), chat.NewUserMessage(chat.NewTextPart("u2b")),
	}
	putRun(t, rt, sess.ID, "run_1", 100, 3)
	putRun(t, rt, sess.ID, "run_2", 200, 6)
	putUserItem(t, rt, sess.ID, "run_1", "item_u1", "first prompt")
	putUserItem(t, rt, sess.ID, "run_2", "item_u2", "second prompt")

	out, err := s.RollbackSession(ctx, protocol.RollbackSessionRequest{SessionID: sess.ID, ToRunID: "run_1"})
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if n := len(rt.history[sess.ID]); n != 3 {
		t.Fatalf("messages = %d, want truncated to 3 (run_1 watermark)", n)
	}
	if len(out.DroppedRuns) != 1 || out.DroppedRuns[0].Run.ID != "run_2" {
		t.Fatalf("droppedRuns = %+v, want [run_2]", out.DroppedRuns)
	}
	if ui := out.DroppedRuns[0].UserInput; len(ui) != 1 || ui[0].Text != "second prompt" {
		t.Fatalf("dropped userInput = %+v, want 'second prompt'", ui)
	}
	// run_2's durable history is gone; run_1 survives.
	_, runs, _ := rt.hist.List(ctx, sess.ID)
	if len(runs) != 1 || runs[0].ID != "run_1" {
		t.Fatalf("surviving runs = %+v, want [run_1]", runs)
	}
}

func TestRollbackSession_CancelsDroppedParkedRun(t *testing.T) {
	s, rt := rollbackHarness(t)
	ctx := context.Background()
	sess, _ := rt.sess.Create(ctx, "s", "/w")

	rt.history[sess.ID] = []chat.Message{
		chat.NewUserMessage(chat.NewTextPart("u1")), chat.NewAssistantMessage(chat.NewTextPart("a1")),
		chat.NewUserMessage(chat.NewTextPart("u2")), chat.NewAssistantMessage(chat.NewTextPart("a2")),
	}
	putRun(t, rt, sess.ID, "run_1", 100, 2)
	putRun(t, rt, sess.ID, "run_2", 200, 4)
	putUserItem(t, rt, sess.ID, "run_2", "item_u2", "second prompt")
	if err := rt.interrupts.Put(ctx, interrupts.Pending{
		RunID:     "run_2",
		SessionID: sess.ID,
		TurnID:    "turn_parked",
	}); err != nil {
		t.Fatalf("seed interrupt: %v", err)
	}
	turns := &recordingTurns{}
	rt.turns = turns

	out, err := s.RollbackSession(ctx, protocol.RollbackSessionRequest{SessionID: sess.ID, ToRunID: "run_1"})
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if len(out.DroppedRuns) != 1 || out.DroppedRuns[0].Run.ID != "run_2" {
		t.Fatalf("droppedRuns = %+v, want [run_2]", out.DroppedRuns)
	}
	if len(turns.canceled) != 1 {
		t.Fatalf("canceled = %+v, want one parked turn", turns.canceled)
	}
	if got := turns.canceled[0]; got.SessionID != sess.ID || got.TurnID != "turn_parked" {
		t.Fatalf("canceled handle = %+v, want %s/turn_parked", got, sess.ID)
	}
	if pending, _ := rt.interrupts.List(ctx, sess.ID); len(pending) != 0 {
		t.Fatalf("pending interrupts = %+v, want cleared", pending)
	}
}

// TestRollbackSession_DropAll clears the session (omit toRunId) and purges the
// subagent child sessions it spawned (boundary zero → all children).
func TestRollbackSession_DropAll(t *testing.T) {
	s, rt := rollbackHarness(t)
	ctx := context.Background()
	sess, _ := rt.sess.Create(ctx, "s", "/w")
	now := time.Now().UTC()
	child, _ := rt.sess.SaveSubtask(ctx, session.Subtask{
		ID: "ses_child", ParentID: sess.ID, AgentName: "subtask-agent", StartedAt: now, UpdatedAt: now,
	})
	rt.history[sess.ID] = []chat.Message{chat.NewUserMessage(chat.NewTextPart("u1")), chat.NewAssistantMessage(chat.NewTextPart("a1"))}
	rt.history[child.ID] = []chat.Message{chat.NewUserMessage(chat.NewTextPart("sub"))}
	putRun(t, rt, sess.ID, "run_1", 100, 2)
	if err := rt.interrupts.Put(ctx, interrupts.Pending{RunID: "run_child", SessionID: child.ID}); err != nil {
		t.Fatalf("seed child interrupt: %v", err)
	}

	out, err := s.RollbackSession(ctx, protocol.RollbackSessionRequest{SessionID: sess.ID})
	if err != nil {
		t.Fatalf("rollback all: %v", err)
	}
	if len(out.DroppedRuns) != 1 {
		t.Fatalf("droppedRuns = %d, want 1", len(out.DroppedRuns))
	}
	if _, ok := rt.history[sess.ID]; ok {
		t.Fatal("session messages must be cleared on drop-all")
	}
	// The subagent child session and its messages are purged.
	if _, err := rt.sess.Get(ctx, child.ID); err == nil {
		t.Fatal("subagent child session must be purged on drop-all")
	}
	if _, ok := rt.history[child.ID]; ok {
		t.Fatal("subagent child messages must be purged")
	}
	if pending, _ := rt.interrupts.List(ctx, child.ID); len(pending) != 0 {
		t.Fatalf("subagent child interrupts = %+v, want purged", pending)
	}
}

func TestRollbackSessionPreservesUserForks(t *testing.T) {
	s, rt := rollbackHarness(t)
	ctx := t.Context()
	parent, err := rt.sess.Create(ctx, "parent", t.TempDir())
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	fork, err := rt.sess.Fork(ctx, parent.ID, "")
	if err != nil {
		t.Fatalf("create fork: %v", err)
	}
	rt.history[fork.ID] = []chat.Message{chat.NewUserMessage(chat.NewTextPart("keep me"))}
	putRun(t, rt, parent.ID, "run_1", 100, 0)

	if _, err := s.RollbackSession(ctx, protocol.RollbackSessionRequest{SessionID: parent.ID}); err != nil {
		t.Fatalf("rollback all: %v", err)
	}
	if _, err := rt.sess.Get(ctx, fork.ID); err != nil {
		t.Fatalf("user fork was deleted by parent rollback: %v", err)
	}
	if len(rt.history[fork.ID]) != 1 {
		t.Fatalf("fork history = %+v, want preserved", rt.history[fork.ID])
	}
}

// TestRollbackSession_Busy refuses to roll back while a run is in flight.
func TestRollbackSession_Busy(t *testing.T) {
	s, rt := rollbackHarness(t)
	ctx := context.Background()
	sess, _ := rt.sess.Create(ctx, "s", "/w")
	s.coordinator.ClaimSession(sess.ID) // simulate a run in flight (admission slot held)

	if _, err := s.RollbackSession(ctx, protocol.RollbackSessionRequest{SessionID: sess.ID}); !errors.Is(err, protocol.ErrSessionBusy) {
		t.Fatalf("rollback under live run = %v, want ErrSessionBusy", err)
	}
}

// TestPersistRunCarriesCreatedAt guards rollback boundary ordering on the
// canonical transcript model; there is no wire blob to decode or replace.
func TestPersistRunCarriesCreatedAt(t *testing.T) {
	_, rt := rollbackHarness(t)
	ctx := context.Background()
	sess, _ := rt.sess.Create(ctx, "s", "/w")

	started := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)

	outcome := execution.OutcomeCompleted
	commit := appRuns.EventCommit{
		RunID: "run_1", SessionID: sess.ID, State: appRuns.StateTerminalize, Outcome: outcome,
		Run: &transcript.Run{
			ID: "run_1", SessionID: sess.ID, State: execution.Completed, Outcome: &outcome,
			CreatedAt: started, UpdatedAt: started.Add(time.Minute), MessageMark: -1,
		},
	}
	if err := rt.RunSegmentEffects(nil, nil).CommitEvent(ctx, commit); err != nil {
		t.Fatalf("commit terminal run: %v", err)
	}

	_, runs, err := rt.hist.List(ctx, sess.ID)
	if err != nil || len(runs) != 1 {
		t.Fatalf("list runs = %d (err %v), want 1", len(runs), err)
	}
	if runs[0].CreatedAt.IsZero() {
		t.Fatal("terminal run persisted CreatedAt as zero — rollback boundary math would over-purge")
	}
	if !runs[0].CreatedAt.Equal(started) {
		t.Errorf("CreatedAt = %v, want the run's start %v", runs[0].CreatedAt, started)
	}
}

// TestClaimSession is the single-writer-per-session admission guard that closes
// the runs.start / runs.resume check-then-register TOCTOU: a second claim is
// rejected while the first is outstanding (even though no run is in s.runs yet),
// a claimed session reads as active to the rollback/start busy check, and a
// release reopens the slot.
func TestClaimSession(t *testing.T) {
	s, _ := rollbackHarness(t)
	if !s.coordinator.ClaimSession("s1") {
		t.Fatal("first claim must succeed")
	}
	if s.coordinator.ClaimSession("s1") {
		t.Fatal("second claim on the same session must fail while the first is outstanding")
	}
	if !s.coordinator.ClaimSession("s2") {
		t.Fatal("a different session must claim independently")
	}
	if !s.hasActiveRun("s1") {
		t.Fatal("a claimed (not-yet-registered) session must read as active")
	}
	s.coordinator.ReleaseSession("s1")
	if s.hasActiveRun("s1") {
		t.Fatal("a released session must no longer read as active")
	}
	if !s.coordinator.ClaimSession("s1") {
		t.Fatal("claim must succeed again after release")
	}
}

// TestForkSession_FromRun truncate-copies the parent's history up to and
// including the named run's watermark into the child.
func TestForkSession_FromRun(t *testing.T) {
	s, rt := rollbackHarness(t)
	ctx := context.Background()
	parent, _ := rt.sess.Create(ctx, "p", "/w")
	rt.history[parent.ID] = []chat.Message{
		chat.NewUserMessage(chat.NewTextPart("u1")), chat.NewAssistantMessage(chat.NewTextPart("a1")),
		chat.NewUserMessage(chat.NewTextPart("u2")), chat.NewAssistantMessage(chat.NewTextPart("a2")),
	}
	putRun(t, rt, parent.ID, "run_1", 100, 2)
	putRun(t, rt, parent.ID, "run_2", 200, 4)

	child, err := s.ForkSession(ctx, protocol.ForkSessionRequest{SessionID: parent.ID, FromRunID: "run_1"})
	if err != nil {
		t.Fatalf("fork fromRun: %v", err)
	}
	if n := len(rt.history[child.ID]); n != 2 {
		t.Fatalf("child history = %d, want 2 (run_1 watermark, inclusive)", n)
	}
}
