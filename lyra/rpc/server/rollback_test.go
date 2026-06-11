package server

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/model/chat"
	historysvc "github.com/Tangerg/lynx/lyra/internal/service/history"
	"github.com/Tangerg/lynx/lyra/internal/storage/sqlite"
	"github.com/Tangerg/lynx/lyra/rpc/protocol"
)

// root / cont build timeline records (the unexported runRecord) directly so the
// boundary math can be audited without a runtime.
func root(id string, atUnix int64, mark int) runRecord {
	return runRecord{ref: protocol.RunRef{ID: id, CreatedAt: time.Unix(atUnix, 0).UTC()}, mark: mark}
}
func cont(id, parent string, atUnix int64, mark int) runRecord {
	return runRecord{ref: protocol.RunRef{ID: id, ParentRunID: parent, CreatedAt: time.Unix(atUnix, 0).UTC()}, mark: mark}
}

// TestBoundaryAt audits the inclusive-keep split: a kept root keeps its own
// continuation chain (so the watermark is the chain terminal's, not the root's),
// the drop set is everything from the next root on, drop-all keeps nothing, and
// a non-root / unknown target errors (rollback's requireRoot).
func TestBoundaryAt(t *testing.T) {
	// R1 @1 mark2 → C1 (resume of R1) @2 mark4 → R2 @3 mark6 → R3 @4 mark9
	tl := runTimeline{
		root("R1", 1, 2),
		cont("C1", "R1", 2, 4),
		root("R2", 3, 6),
		root("R3", 4, 9),
	}

	// Keep through R1 inclusive → keep R1+C1 (watermark 4, the chain terminal),
	// drop R2+R3, boundary at R2's time.
	b, err := tl.boundaryAt("R1", true)
	if err != nil {
		t.Fatalf("R1: %v", err)
	}
	if b.KeepMark != 4 || len(b.Dropped) != 2 || b.Dropped[0].ref.ID != "R2" || !b.BoundaryTime.Equal(time.Unix(3, 0).UTC()) {
		t.Fatalf("R1 split = keep%d drop%v boundary%v, want keep4 [R2 R3] @3", b.KeepMark, runIDs(b.Dropped), b.BoundaryTime.Unix())
	}

	// Keep through R2 → watermark 6, drop only R3.
	if b, _ := tl.boundaryAt("R2", true); b.KeepMark != 6 || len(b.Dropped) != 1 || b.Dropped[0].ref.ID != "R3" {
		t.Fatalf("R2 split = keep%d drop%v, want keep6 [R3]", b.KeepMark, runIDs(b.Dropped))
	}

	// Keep through the latest root → nothing to drop.
	if b, _ := tl.boundaryAt("R3", true); len(b.Dropped) != 0 {
		t.Fatalf("R3 drop = %v, want none", runIDs(b.Dropped))
	}

	// Drop everything (empty target) → keep 0, drop all.
	if b, _ := tl.boundaryAt("", true); b.KeepMark != 0 || len(b.Dropped) != 4 || !b.BoundaryTime.IsZero() {
		t.Fatalf("drop-all = keep%d drop%d boundary%v, want keep0 drop4 zero", b.KeepMark, len(b.Dropped), b.BoundaryTime)
	}

	// A continuation target is not a root → invalid_params (rollback).
	if _, err := tl.boundaryAt("C1", true); !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("C1 err = %v, want ErrInvalidParams", err)
	}
	// Unknown target → run_not_found.
	if _, err := tl.boundaryAt("ghost", true); !errors.Is(err, protocol.ErrRunNotFound) {
		t.Fatalf("ghost err = %v, want ErrRunNotFound", err)
	}
	// Fork is lax: a continuation target is allowed (requireRoot=false).
	if _, err := tl.boundaryAt("C1", false); err != nil {
		t.Fatalf("C1 lax err = %v, want nil", err)
	}
}

func runIDs(recs []runRecord) []string {
	out := make([]string, len(recs))
	for i, r := range recs {
		out[i] = r.ref.ID
	}
	return out
}

// rollbackHarness wires a Server over a sqlite-backed stub: a real session
// service + history + interrupt store, plus the in-memory chat-memory map.
func rollbackHarness(t *testing.T) (*Server, *stubRuntime) {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	rt := &stubRuntime{
		sess:       sqlite.NewSessionService(db),
		model:      "default-model",
		history:    map[string][]chat.Message{},
		hist:       sqlite.NewHistoryStore(db),
		interrupts: sqlite.NewInterruptStore(db),
	}
	return &Server{rt: rt, runs: map[string]*runEntry{}}, rt
}

func putRun(t *testing.T, rt *stubRuntime, sessionID, runID, parentRunID string, atUnix int64, mark int) {
	t.Helper()
	ref := protocol.RunRef{ID: runID, SessionID: sessionID, ParentRunID: parentRunID, CreatedAt: time.Unix(atUnix, 0).UTC()}
	blob, _ := json.Marshal(ref)
	if err := rt.hist.PutRun(context.Background(), historysvc.Run{
		SessionID: sessionID, RunID: runID, UpdatedAt: time.Unix(atUnix, 0).UTC(), Blob: blob, Mark: mark,
	}); err != nil {
		t.Fatalf("putRun %s: %v", runID, err)
	}
}

func putUserItem(t *testing.T, rt *stubRuntime, sessionID, runID, itemID, text string) {
	t.Helper()
	item := protocol.Item{ID: itemID, RunID: runID, Type: protocol.ItemTypeUserMessage, Content: []protocol.ContentBlock{{Type: "text", Text: text}}}
	blob, _ := json.Marshal(item)
	if err := rt.hist.AppendItem(context.Background(), historysvc.Item{
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
}

// TestRollbackSession_Busy refuses to roll back while a run is in flight.
func TestRollbackSession_Busy(t *testing.T) {
	s, rt := rollbackHarness(t)
	ctx := context.Background()
	sess, _ := rt.sess.Create(ctx, "s", "/w")
	s.runs["run_live"] = &runEntry{runID: "run_live", sessionID: sess.ID}

	if _, err := s.RollbackSession(ctx, protocol.RollbackSessionRequest{SessionID: sess.ID}); !errors.Is(err, protocol.ErrSessionBusy) {
		t.Fatalf("rollback under live run = %v, want ErrSessionBusy", err)
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
