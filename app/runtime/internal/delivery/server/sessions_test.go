package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/workspacepath"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
	"github.com/Tangerg/lynx/core/chat"
)

func TestUpdateSession(t *testing.T) {
	s, svc := newSessionServer(t)
	ctx := context.Background()
	created, _ := svc.Create(ctx, "old", "/w")

	title := "new title"
	out, err := s.UpdateSession(ctx, protocol.UpdateSessionRequest{SessionID: created.ID, Title: &title})
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if out.Title != "new title" {
		t.Errorf("Title = %q, want %q", out.Title, "new title")
	}

	// model edit routes to SetModel and surfaces on the wire
	model := "claude-opus-4-8"
	out, err = s.UpdateSession(ctx, protocol.UpdateSessionRequest{SessionID: created.ID, Model: &model})
	if err != nil {
		t.Fatalf("set model: %v", err)
	}
	if out.Model != model {
		t.Errorf("Model = %q, want %q", out.Model, model)
	}

	// whitespace-only title → invalid_params (a session title must be non-empty)
	blank := "   "
	if _, err := s.UpdateSession(ctx, protocol.UpdateSessionRequest{SessionID: created.ID, Title: &blank}); !errors.Is(err, protocol.ErrInvalidParams) {
		t.Errorf("blank title err = %v, want ErrInvalidParams", err)
	}

	// unknown id → session_not_found
	if _, err := s.UpdateSession(ctx, protocol.UpdateSessionRequest{SessionID: "nope", Title: &title}); !errors.Is(err, protocol.ErrSessionNotFound) {
		t.Errorf("unknown id err = %v, want ErrSessionNotFound", err)
	}

	// relocate to a non-existent dir → cwd_unavailable (a stale path would
	// silently break later runs)
	ghost := "/no/such/dir"
	if _, err := s.UpdateSession(ctx, protocol.UpdateSessionRequest{SessionID: created.ID, Cwd: &ghost}); !errors.Is(err, protocol.ErrCwdUnavailable) {
		t.Errorf("relocate to ghost err = %v, want ErrCwdUnavailable", err)
	}

	// relocate to a real dir → cwd surfaces on the wire
	newCwd := t.TempDir()
	out, err = s.UpdateSession(ctx, protocol.UpdateSessionRequest{SessionID: created.ID, Cwd: &newCwd})
	if err != nil {
		t.Fatalf("relocate: %v", err)
	}
	if out.Cwd != workspacepath.Canonical(newCwd) {
		t.Errorf("Cwd = %q, want relocated %q", out.Cwd, workspacepath.Canonical(newCwd))
	}
	if !s.coordinator.ClaimSession(created.ID) {
		t.Fatal("claim active session")
	}
	busyCwd := t.TempDir()
	if _, err := s.UpdateSession(ctx, protocol.UpdateSessionRequest{SessionID: created.ID, Cwd: &busyCwd}); !errors.Is(err, protocol.ErrSessionBusy) {
		t.Fatalf("relocate under active run = %v, want ErrSessionBusy", err)
	}
	s.coordinator.ReleaseSession(created.ID)

	// metadata is full-replaced and round-trips arbitrary JSON values
	meta := map[string]any{"pinned": true, "n": float64(3)}
	out, err = s.UpdateSession(ctx, protocol.UpdateSessionRequest{SessionID: created.ID, Metadata: &meta})
	if err != nil {
		t.Fatalf("set metadata: %v", err)
	}
	if out.Metadata["pinned"] != true || out.Metadata["n"] != float64(3) {
		t.Errorf("Metadata = %+v, want {pinned:true, n:3}", out.Metadata)
	}
}

// TestDeleteSession_Cascade verifies a deleted session takes its session-scoped
// data with it: transcript runs+items, conversation messages, and open
// interrupts. Without the cascade the sessions row vanishes but those rows
// orphan (the bug: items.list / runs.listOpenInterrupts kept resolving a
// deleted session).
func TestDeleteSession_Cascade(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	svc := sqlite.NewSessionStore(db)
	hist := sqlite.NewTranscriptStore(db)
	ints := sqlite.NewInterruptStore(db)
	created, _ := svc.Create(ctx, "doomed", "/w")
	id := created.ID
	now := time.Now().UTC()
	if _, err := svc.SaveSubtask(ctx, session.Subtask{
		ID: "ses_subtask", ParentID: id, AgentName: "subtask-agent", StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed subtask: %v", err)
	}
	fork, err := svc.Fork(ctx, id, "")
	if err != nil {
		t.Fatalf("seed user fork: %v", err)
	}

	// Seed one of every session-scoped row.
	if err := hist.PutRun(ctx, transcript.Run{SessionID: id, ID: "run_1"}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := hist.AppendItem(ctx, transcript.Item{SessionID: id, RunID: "run_1", ID: "item_1"}); err != nil {
		t.Fatalf("seed item: %v", err)
	}
	if err := ints.Put(ctx, interrupts.Pending{RunID: "run_1", SessionID: id}); err != nil {
		t.Fatalf("seed interrupt: %v", err)
	}
	history := map[string][]chat.Message{id: {chat.NewUserMessage(chat.NewTextPart("hi"))}}

	s := newTestServer(&stubRuntime{sess: svc, hist: hist, interrupts: ints, history: history})
	if err := s.DeleteSession(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if _, err := svc.Get(ctx, id); !errors.Is(err, session.ErrNotFound) {
		t.Errorf("session still present after delete: err = %v", err)
	}
	if _, err := svc.Get(ctx, "ses_subtask"); !errors.Is(err, session.ErrNotFound) {
		t.Errorf("owned subtask still present after parent delete: err = %v", err)
	}
	if _, err := svc.Get(ctx, fork.ID); err != nil {
		t.Errorf("independent user fork was deleted with its parent: %v", err)
	}
	if items, runs, _ := hist.List(ctx, id); len(items) != 0 || len(runs) != 0 {
		t.Errorf("transcript not cascaded: %d items, %d runs left", len(items), len(runs))
	}
	if pending, _ := ints.List(ctx, id); len(pending) != 0 {
		t.Errorf("interrupts not cascaded: %d left", len(pending))
	}
	if _, ok := history[id]; ok {
		t.Errorf("conversation messages not cascaded: still present")
	}
	if s.hasActiveRun(id) {
		t.Fatal("delete leaked the session mutation claim")
	}
}

func TestDeleteSession_RejectsActiveSession(t *testing.T) {
	s, svc := newSessionServer(t)
	ctx := context.Background()
	created, err := svc.Create(ctx, "live", "/w")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !s.coordinator.ClaimSession(created.ID) {
		t.Fatal("claim session")
	}
	t.Cleanup(func() { s.coordinator.ReleaseSession(created.ID) })

	if err := s.DeleteSession(ctx, created.ID); !errors.Is(err, protocol.ErrSessionBusy) {
		t.Fatalf("delete under active claim = %v, want ErrSessionBusy", err)
	}
	if _, err := svc.Get(ctx, created.ID); err != nil {
		t.Fatalf("session mutated under active claim: %v", err)
	}
}

func TestDeleteSession_CancelsParkedTurn(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	svc := sqlite.NewSessionStore(db)
	hist := sqlite.NewTranscriptStore(db)
	ints := sqlite.NewInterruptStore(db)
	created, _ := svc.Create(ctx, "parked", "/w")
	id := created.ID
	if err := ints.Put(ctx, interrupts.Pending{
		RunID:     "run_parked",
		SessionID: id,
		TurnID:    "turn_parked",
	}); err != nil {
		t.Fatalf("seed interrupt: %v", err)
	}

	turns := &recordingTurns{}
	s := newTestServer(&stubRuntime{sess: svc, hist: hist, interrupts: ints, history: map[string][]chat.Message{}, turns: turns})
	if err := s.DeleteSession(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if len(turns.canceled) != 1 {
		t.Fatalf("canceled = %+v, want one parked turn", turns.canceled)
	}
	if got := turns.canceled[0]; got.SessionID != id || got.TurnID != "turn_parked" {
		t.Fatalf("canceled handle = %+v, want %s/turn_parked", got, id)
	}
	if pending, _ := ints.List(ctx, id); len(pending) != 0 {
		t.Fatalf("pending interrupts = %+v, want cleared", pending)
	}
}

// TestForkSession: a full-history fork inherits the parent's cwd, copies its
// history into the child, and honors a title override; a run-boundary fork
// (fromRunId) against an unknown run is run_not_found.
func TestForkSession(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	svc := sqlite.NewSessionStore(db)
	ctx := context.Background()
	parent, _ := svc.Create(ctx, "research", "/work/proj")

	hist := map[string][]chat.Message{parent.ID: {chat.NewUserMessage(chat.NewTextPart("hello")), chat.NewAssistantMessage(chat.NewTextPart("hi"))}}
	s := newTestServer(&stubRuntime{sess: svc, history: hist, hist: sqlite.NewTranscriptStore(db)})

	child, err := s.ForkSession(ctx, protocol.ForkSessionRequest{SessionID: parent.ID, Title: "branch A"})
	if err != nil {
		t.Fatalf("fork: %v", err)
	}
	if child.Cwd != "/work/proj" {
		t.Errorf("child cwd = %q, want inherited /work/proj", child.Cwd)
	}
	if child.Title != "branch A" {
		t.Errorf("child title = %q, want override 'branch A'", child.Title)
	}
	if got := len(hist[child.ID]); got != 0 {
		t.Errorf("child history = %d msgs, want 0 without a terminal run boundary", got)
	}

	// run-boundary fork against a run that doesn't exist → run_not_found
	if _, err := s.ForkSession(ctx, protocol.ForkSessionRequest{SessionID: parent.ID, FromRunID: "run_x"}); !errors.Is(err, protocol.ErrRunNotFound) {
		t.Errorf("fromRunId fork err = %v, want ErrRunNotFound", err)
	}

	// unknown parent → session_not_found
	if _, err := s.ForkSession(ctx, protocol.ForkSessionRequest{SessionID: "nope"}); !errors.Is(err, protocol.ErrSessionNotFound) {
		t.Errorf("unknown parent err = %v, want ErrSessionNotFound", err)
	}
}
