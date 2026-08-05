package server

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
	"github.com/Tangerg/lynx/core/chat"
)

func TestUpdateSession(t *testing.T) {
	s, svc, rt := newSessionServer(t)
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

	// relocate to a non-existent dir → workspace_unavailable (a stale path would
	// silently break later runs)
	ghost := "/no/such/dir"
	if _, err := s.UpdateSession(ctx, protocol.UpdateSessionRequest{
		SessionID: created.ID, Workspace: &protocol.WorkspaceRef{Path: ghost},
	}); !errors.Is(err, protocol.ErrWorkspaceUnavailable) {
		t.Errorf("relocate to ghost err = %v, want ErrWorkspaceUnavailable", err)
	}

	// relocate to a real dir → cwd surfaces on the wire
	newCWD := t.TempDir()
	out, err = s.UpdateSession(ctx, protocol.UpdateSessionRequest{
		SessionID: created.ID, Workspace: &protocol.WorkspaceRef{Path: newCWD},
	})
	if err != nil {
		t.Fatalf("relocate: %v", err)
	}
	if out.Workspace.Ref.Path != canonicalWorkspacePath(t, newCWD) {
		t.Errorf("workspace = %q, want relocated %q", out.Workspace.Ref.Path, canonicalWorkspacePath(t, newCWD))
	}
	releaseSession, ok := rt.admissions.AcquireSession(created.ID)
	if !ok {
		t.Fatal("claim active session")
	}
	busyCWD := t.TempDir()
	if _, err := s.UpdateSession(ctx, protocol.UpdateSessionRequest{
		SessionID: created.ID, Workspace: &protocol.WorkspaceRef{Path: busyCWD},
	}); !errors.Is(err, protocol.ErrSessionBusy) {
		t.Fatalf("relocate under active run = %v, want ErrSessionBusy", err)
	}
	releaseSession()

	if err := os.RemoveAll(newCWD); err != nil {
		t.Fatalf("remove cwd: %v", err)
	}
	out, err = s.GetSession(ctx, created.ID)
	if err != nil {
		t.Fatalf("get session with missing cwd: %v", err)
	}
	if out.Workspace.Availability != protocol.WorkspaceMissing || out.Workspace.ProjectRoot != out.Workspace.Ref.Path {
		t.Fatalf("missing workspace projection = %+v", out)
	}
}

// TestDeleteSession_Cascade verifies a deleted session takes its session-scoped
// data with it: transcript runs+items, conversation messages, and open
// interrupts. Without the cascade the sessions row vanishes but those rows
// orphan (the bug: items.list / runs.listOpenInterrupts kept resolving a
// deleted session).
func TestDeleteSession_Cascade(t *testing.T) {
	db, err := sqlite.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	svc := sqlite.NewSessionStore(db)
	hist := sqlite.NewTranscriptStore(db)
	runStore := sqlite.NewRunStore(db)
	ints := sqlite.NewInterruptStore(db)
	created, _ := svc.Create(ctx, "doomed", "/w")
	id := created.ID
	now := time.Now().UTC()
	fork, err := svc.Fork(ctx, id)
	if err != nil {
		t.Fatalf("seed user fork: %v", err)
	}

	// Seed one of every session-scoped row.
	if err := runStore.Admit(ctx, execution.RunDraft{SegmentID: "seg_open", RunID: "run_1", SessionID: id, CreatedAt: now}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := hist.AppendItem(ctx, transcript.Item{SessionID: id, RunID: "run_1", ID: "item_1", OccurredAt: now}); err != nil {
		t.Fatalf("seed item: %v", err)
	}
	if err := ints.Open(ctx, serverPending("run_1", id, "", "", nil, now)); err != nil {
		t.Fatalf("seed interrupt: %v", err)
	}
	history := map[string][]chat.Message{id: {chat.NewUserMessage(chat.NewTextPart("hi"))}}

	runtime := &stubRuntime{sess: svc, hist: hist, runs: runStore, interrupts: ints, history: history}
	s := newTestServer(runtime)
	if err := s.DeleteSession(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if _, err := svc.Get(ctx, id); !errors.Is(err, session.ErrNotFound) {
		t.Errorf("session still present after delete: err = %v", err)
	}
	if _, err := svc.Get(ctx, fork.ID); err != nil {
		t.Errorf("independent user fork was deleted with its parent: %v", err)
	}
	runsLeft, _ := runtime.runs.ListRuns(ctx, id)
	if items, _ := hist.List(ctx, id); len(items) != 0 || len(runsLeft) != 0 {
		t.Errorf("transcript not cascaded: %d items, %d runs left", len(items), len(runsLeft))
	}
	if pending, _ := ints.List(ctx, id); len(pending) != 0 {
		t.Errorf("interrupts not cascaded: %d left", len(pending))
	}
	if _, ok := history[id]; ok {
		t.Errorf("conversation messages not cascaded: still present")
	}
	if runtime.admissions.ActiveSessions()[id] {
		t.Fatal("delete leaked the session mutation claim")
	}
}

func TestDeleteSession_RejectsActiveSession(t *testing.T) {
	s, svc, rt := newSessionServer(t)
	ctx := context.Background()
	created, err := svc.Create(ctx, "live", "/w")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	releaseSession, ok := rt.admissions.AcquireSession(created.ID)
	if !ok {
		t.Fatal("claim session")
	}
	t.Cleanup(releaseSession)

	if err := s.DeleteSession(ctx, created.ID); !errors.Is(err, protocol.ErrSessionBusy) {
		t.Fatalf("delete under active claim = %v, want ErrSessionBusy", err)
	}
	if _, err := svc.Get(ctx, created.ID); err != nil {
		t.Fatalf("session mutated under active claim: %v", err)
	}
}

func TestDeleteSession_CancelsParkedTurn(t *testing.T) {
	db, err := sqlite.Open(t.Context(), ":memory:")
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
	if err := ints.Open(ctx, serverPending(
		"run_parked",
		id,
		"exec_parked",
		"process_parked",
		nil,
		time.Now().UTC(),
	)); err != nil {
		t.Fatalf("seed interrupt: %v", err)
	}

	executions := &recordingExecutions{}
	s := newTestServer(&stubRuntime{sess: svc, hist: hist, runs: sqlite.NewRunStore(db), interrupts: ints, history: map[string][]chat.Message{}, execution: executions})
	if err := s.DeleteSession(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if len(executions.canceled) != 1 {
		t.Fatalf("canceled = %+v, want one parked execution", executions.canceled)
	}
	if got := executions.canceled[0]; got.SessionID != id || got.ExecutorID != "exec_parked" {
		t.Fatalf("canceled execution = %+v, want %s/exec_parked", got, id)
	}
	if pending, _ := ints.List(ctx, id); len(pending) != 0 {
		t.Fatalf("pending interrupts = %+v, want cleared", pending)
	}
}

// TestForkSession: a full-history fork inherits the parent's cwd, copies its
// history into the child, and honors a title override; a run-boundary fork
// (fromRunId) against an unknown run is run_not_found.
func TestForkSession(t *testing.T) {
	db, err := sqlite.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	svc := sqlite.NewSessionStore(db)
	ctx := context.Background()
	parent, _ := svc.Create(ctx, "research", "/work/proj")

	hist := map[string][]chat.Message{parent.ID: {chat.NewUserMessage(chat.NewTextPart("hello")), chat.NewAssistantMessage(chat.NewTextPart("hi"))}}
	s := newTestServer(&stubRuntime{sess: svc, history: hist, hist: sqlite.NewTranscriptStore(db), runs: sqlite.NewRunStore(db)})

	child, err := s.ForkSession(ctx, protocol.ForkSessionRequest{SessionID: parent.ID, Title: "branch A"})
	if err != nil {
		t.Fatalf("fork: %v", err)
	}
	if child.Workspace.Ref.Path != "/work/proj" {
		t.Errorf("child workspace = %q, want inherited /work/proj", child.Workspace.Ref.Path)
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
