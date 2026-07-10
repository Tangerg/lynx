package server

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
	"github.com/Tangerg/lynx/core/model/chat"
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
	}
	return newTestServer(rt), rt
}

func putRun(t *testing.T, rt *stubRuntime, sessionID, runID, parentRunID string, atUnix int64, mark int) {
	t.Helper()
	ref := protocol.RunRef{ID: runID, SessionID: sessionID, ParentRunID: parentRunID, CreatedAt: time.Unix(atUnix, 0).UTC()}
	blob, _ := json.Marshal(ref)
	if err := rt.hist.PutRun(context.Background(), transcript.Run{
		SessionID: sessionID, RunID: runID, UpdatedAt: time.Unix(atUnix, 0).UTC(), Blob: blob, Mark: mark,
	}); err != nil {
		t.Fatalf("putRun %s: %v", runID, err)
	}
}

func putUserItem(t *testing.T, rt *stubRuntime, sessionID, runID, itemID, text string) {
	t.Helper()
	item := protocol.Item{ID: itemID, RunID: runID, Type: protocol.ItemTypeUserMessage, Content: []protocol.ContentBlock{{Type: "text", Text: text}}}
	blob, _ := json.Marshal(item)
	if err := rt.hist.AppendItem(context.Background(), transcript.Item{
		SessionID: sessionID, RunID: runID, ItemID: itemID, CreatedAt: time.Unix(1, 0).UTC(), Blob: blob,
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
		chat.NewUserMessage("u1"), chat.NewAssistantMessage("a1"), chat.NewUserMessage("u1b"),
		chat.NewUserMessage("u2"), chat.NewAssistantMessage("a2"), chat.NewUserMessage("u2b"),
	}
	putRun(t, rt, sess.ID, "run_1", "", 100, 3)
	putRun(t, rt, sess.ID, "run_2", "", 200, 6)
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
	if len(runs) != 1 || runs[0].RunID != "run_1" {
		t.Fatalf("surviving runs = %+v, want [run_1]", runs)
	}
}

func TestRollbackSession_CancelsDroppedParkedRun(t *testing.T) {
	s, rt := rollbackHarness(t)
	ctx := context.Background()
	sess, _ := rt.sess.Create(ctx, "s", "/w")

	rt.history[sess.ID] = []chat.Message{
		chat.NewUserMessage("u1"), chat.NewAssistantMessage("a1"),
		chat.NewUserMessage("u2"), chat.NewAssistantMessage("a2"),
	}
	putRun(t, rt, sess.ID, "run_1", "", 100, 2)
	putRun(t, rt, sess.ID, "run_2", "", 200, 4)
	putUserItem(t, rt, sess.ID, "run_2", "item_u2", "second prompt")
	if err := rt.interrupts.Put(ctx, interrupts.Pending{
		ParentRunID: "run_2",
		SessionID:   sess.ID,
		TurnID:      "turn_parked",
		Interrupts:  []byte(`[]`),
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
	child, _ := rt.sess.CreateSubtask(ctx, "ses_child", sess.ID)
	rt.history[sess.ID] = []chat.Message{chat.NewUserMessage("u1"), chat.NewAssistantMessage("a1")}
	rt.history[child.ID] = []chat.Message{chat.NewUserMessage("sub")}
	putRun(t, rt, sess.ID, "run_1", "", 100, 2)
	if err := rt.interrupts.Put(ctx, interrupts.Pending{ParentRunID: "run_child", SessionID: child.ID, Interrupts: []byte(`[]`)}); err != nil {
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

// TestRollbackSession_Busy refuses to roll back while a run is in flight.
func TestRollbackSession_Busy(t *testing.T) {
	s, rt := rollbackHarness(t)
	ctx := context.Background()
	sess, _ := rt.sess.Create(ctx, "s", "/w")
	s.claimSession(sess.ID) // simulate a run in flight (admission slot held)

	if _, err := s.RollbackSession(ctx, protocol.RollbackSessionRequest{SessionID: sess.ID}); !errors.Is(err, protocol.ErrSessionBusy) {
		t.Fatalf("rollback under live run = %v, want ErrSessionBusy", err)
	}
}

// TestPersistRunCarriesCreatedAt guards the rollback over-purge bug: the
// terminal RunRef synthesized on run.finished replaces the whole stored blob
// (PutRun upsert), so it must carry the run's start CreatedAt. Omitting it
// persisted CreatedAt as zero (json:"createdAt,omitzero"), which collapsed the
// rollback boundary time to the zero time → purgeSubtasksAfter then purged EVERY
// subagent child, including kept runs'. The putRun test helper writes a real
// CreatedAt and so bypassed the production stream side-effect path that exposed
// this — this drives the same side-effect payload the pump hands to
// adapter/runsegment.
func TestPersistRunCarriesCreatedAt(t *testing.T) {
	s, rt := rollbackHarness(t)
	ctx := context.Background()
	sess, _ := rt.sess.Create(ctx, "s", "/w")

	started := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)

	ev := sideEffectEvent("run_1", sess.ID, "", "", protocol.StreamEvent{
		Type:    protocol.StreamRunFinished,
		Outcome: &protocol.RunOutcome{Type: protocol.OutcomeCompleted},
	}, "", "", started)
	s.runSegmentEffects().AfterLive(ctx, ev)

	_, runs, err := rt.hist.List(ctx, sess.ID)
	if err != nil || len(runs) != 1 {
		t.Fatalf("list runs = %d (err %v), want 1", len(runs), err)
	}
	var ref protocol.RunRef
	if err := json.Unmarshal(runs[0].Blob, &ref); err != nil {
		t.Fatalf("decode run blob: %v", err)
	}
	if ref.CreatedAt.IsZero() {
		t.Fatal("terminal RunRef persisted CreatedAt as zero — rollback boundary math would over-purge")
	}
	if !ref.CreatedAt.Equal(started) {
		t.Errorf("CreatedAt = %v, want the run's start %v", ref.CreatedAt, started)
	}
}

// TestClaimSession is the single-writer-per-session admission guard that closes
// the runs.start / runs.resume check-then-register TOCTOU: a second claim is
// rejected while the first is outstanding (even though no run is in s.runs yet),
// a claimed session reads as active to the rollback/start busy check, and a
// release reopens the slot.
func TestClaimSession(t *testing.T) {
	s, _ := rollbackHarness(t)
	if !s.claimSession("s1") {
		t.Fatal("first claim must succeed")
	}
	if s.claimSession("s1") {
		t.Fatal("second claim on the same session must fail while the first is outstanding")
	}
	if !s.claimSession("s2") {
		t.Fatal("a different session must claim independently")
	}
	if !s.hasActiveRun("s1") {
		t.Fatal("a claimed (not-yet-registered) session must read as active")
	}
	s.releaseSession("s1")
	if s.hasActiveRun("s1") {
		t.Fatal("a released session must no longer read as active")
	}
	if !s.claimSession("s1") {
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
		chat.NewUserMessage("u1"), chat.NewAssistantMessage("a1"),
		chat.NewUserMessage("u2"), chat.NewAssistantMessage("a2"),
	}
	putRun(t, rt, parent.ID, "run_1", "", 100, 2)
	putRun(t, rt, parent.ID, "run_2", "", 200, 4)

	child, err := s.ForkSession(ctx, protocol.ForkSessionRequest{SessionID: parent.ID, FromRunID: "run_1"})
	if err != nil {
		t.Fatalf("fork fromRun: %v", err)
	}
	if n := len(rt.history[child.ID]); n != 2 {
		t.Fatalf("child history = %d, want 2 (run_1 watermark, inclusive)", n)
	}
}
