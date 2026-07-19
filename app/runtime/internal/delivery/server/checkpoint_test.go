package server

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
)

// testCheckpointRestorer mirrors the composition root's restorer: it drives the
// checkpoint adapter and maps its disabled/missing-snapshot sentinel onto the
// sessions port sentinel, so a file rollback restores under the coordinator.
type testCheckpointRestorer struct{ cp *workspace.Checkpoints }

func (r testCheckpointRestorer) Restore(ctx context.Context, sessionID, cwd, runID string) error {
	if err := r.cp.Restore(ctx, sessionID, cwd, runID); err != nil {
		switch {
		case errors.Is(err, workspace.ErrCheckpointUnavailable):
			return sessions.ErrCheckpointUnavailable
		case errors.Is(err, workspace.ErrCheckpointRestoreIncomplete):
			return sessions.ErrCheckpointRestoreIncomplete
		}
		return err
	}
	return nil
}

func (r testCheckpointRestorer) DropSession(sessionID string) error {
	return r.cp.DropSession(sessionID)
}

// checkpointHarness extends the rollback harness with a real shadow-git
// checkpoint store and a session whose cwd is a populated temp dir. It returns
// the server, runtime stub, the checkpoint store, session id, and cwd so a test
// can mutate + snapshot. The store is owned by the Host in production, so the
// harness holds it locally and wires it into the sessions coordinator restorer.
func checkpointHarness(t *testing.T) (*Server, *stubRuntime, *workspace.Checkpoints, string, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed; skipping checkpoint rollback test")
	}
	s, rt := rollbackHarness(t)
	cp := workspace.NewCheckpoints(t.TempDir())
	// The restorer lives on the sessions coordinator now, so rebuild it over the
	// real checkpoint store (newTestServer wired a disabled one).
	s.sessions = rt.sessionsCoordinatorWithRestorer(testCheckpointRestorer{cp: cp})
	cwd := t.TempDir()
	// Checkpoints only fire in a real git repo now (Checkpoints.Snapshot's gate,
	// mirroring opencode): a repo's .gitignore is what bounds the whole-tree
	// stage, so a non-repo dir is never snapshotted. Make cwd a repo so the
	// rollback path is exercised.
	if out, err := exec.Command("git", "-C", cwd, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init cwd: %v: %s", err, out)
	}
	ses, err := rt.sess.Create(context.Background(), "ckpt", cwd)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return s, rt, cp, ses.ID, cwd
}

func writeFile(t *testing.T, cwd, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(cwd, name), []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// TestRollback_RestoreBoth restores files AND truncates history: rolling back
// to run1 reverts the working tree to run1's snapshot and drops run2.
func TestRollback_RestoreBoth(t *testing.T) {
	s, rt, cp, sid, cwd := checkpointHarness(t)
	ctx := context.Background()

	writeFile(t, cwd, "a.txt", "v1")
	if err := cp.Snapshot(ctx, sid, cwd, "run1"); err != nil {
		t.Fatalf("snapshot run1: %v", err)
	}
	putRun(t, rt, sid, "run1", 1, 1)

	writeFile(t, cwd, "a.txt", "v2")
	if err := cp.Snapshot(ctx, sid, cwd, "run2"); err != nil {
		t.Fatalf("snapshot run2: %v", err)
	}
	putRun(t, rt, sid, "run2", 2, 2)

	resp, err := s.RollbackSession(ctx, protocol.RollbackSessionRequest{
		SessionID: sid, ToRunID: "run1", RestoreType: protocol.RestoreBoth,
	})
	if err != nil {
		t.Fatalf("rollback both: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(cwd, "a.txt")); string(b) != "v1" {
		t.Errorf("a.txt = %q, want v1 (files restored)", b)
	}
	if len(resp.DroppedRuns) != 1 || resp.DroppedRuns[0].Run.ID != "run2" {
		t.Errorf("droppedRuns = %+v, want [run2]", resp.DroppedRuns)
	}
}

// TestRollback_RestoreFilesKeepsHistory reverts the working tree but drops no
// runs (restoreType=files).
func TestRollback_RestoreFilesKeepsHistory(t *testing.T) {
	s, rt, cp, sid, cwd := checkpointHarness(t)
	ctx := context.Background()

	writeFile(t, cwd, "a.txt", "v1")
	cp.Snapshot(ctx, sid, cwd, "run1")
	putRun(t, rt, sid, "run1", 1, 1)
	writeFile(t, cwd, "a.txt", "v2")
	cp.Snapshot(ctx, sid, cwd, "run2")
	putRun(t, rt, sid, "run2", 2, 2)

	resp, err := s.RollbackSession(ctx, protocol.RollbackSessionRequest{
		SessionID: sid, ToRunID: "run1", RestoreType: protocol.RestoreFiles,
	})
	if err != nil {
		t.Fatalf("rollback files: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(cwd, "a.txt")); string(b) != "v1" {
		t.Errorf("a.txt = %q, want v1", b)
	}
	if len(resp.DroppedRuns) != 0 {
		t.Errorf("droppedRuns = %+v, want none (history kept)", resp.DroppedRuns)
	}
	if pending, _ := rt.muts.ListPending(ctx); len(pending) != 0 {
		t.Fatalf("pending intents after files-only success = %+v, want none", pending)
	}
}

// TestRollback_RestoreBoth_ClearsIntent: a successful files+history rollback
// leaves no pending operation — the §8.5 intent is recorded then cleared, so
// boot recovery has nothing to re-drive.
func TestRollback_RestoreBoth_ClearsIntent(t *testing.T) {
	s, rt, cp, sid, cwd := checkpointHarness(t)
	ctx := context.Background()

	writeFile(t, cwd, "a.txt", "v1")
	if err := cp.Snapshot(ctx, sid, cwd, "run1"); err != nil {
		t.Fatalf("snapshot run1: %v", err)
	}
	putRun(t, rt, sid, "run1", 1, 1)
	writeFile(t, cwd, "a.txt", "v2")
	if err := cp.Snapshot(ctx, sid, cwd, "run2"); err != nil {
		t.Fatalf("snapshot run2: %v", err)
	}
	putRun(t, rt, sid, "run2", 2, 2)

	if _, err := s.RollbackSession(ctx, protocol.RollbackSessionRequest{
		SessionID: sid, ToRunID: "run1", RestoreType: protocol.RestoreBoth,
	}); err != nil {
		t.Fatalf("rollback both: %v", err)
	}
	if pending, _ := rt.muts.ListPending(ctx); len(pending) != 0 {
		t.Fatalf("pending intents after success = %+v, want none (recorded then cleared)", pending)
	}
}

// TestRecoverRollbacks re-drives a rollback a crash left unfinished: an intent is
// logged with the working tree already reverted but the history NOT yet
// truncated (the crash window). Boot recovery restores (reentrant), truncates,
// and clears the intent.
func TestRecoverRollbacks(t *testing.T) {
	s, rt, cp, sid, cwd := checkpointHarness(t)
	ctx := context.Background()

	writeFile(t, cwd, "a.txt", "v1")
	if err := cp.Snapshot(ctx, sid, cwd, "run1"); err != nil {
		t.Fatalf("snapshot run1: %v", err)
	}
	putRun(t, rt, sid, "run1", 1, 1)
	writeFile(t, cwd, "a.txt", "v2")
	if err := cp.Snapshot(ctx, sid, cwd, "run2"); err != nil {
		t.Fatalf("snapshot run2: %v", err)
	}
	putRun(t, rt, sid, "run2", 2, 2)

	// Simulate the crash: the intent is logged but neither resource is rolled back
	// yet (tree still v2, run2 still in history).
	if err := rt.muts.Record(ctx, execution.WorkspaceMutation{
		SessionID: sid, Cwd: cwd, ToRunID: "run1", RestoreHistory: true,
	}); err != nil {
		t.Fatalf("record intent: %v", err)
	}

	if err := s.RecoverRollbacks(ctx); err != nil {
		t.Fatalf("RecoverRollbacks: %v", err)
	}

	if b, _ := os.ReadFile(filepath.Join(cwd, "a.txt")); string(b) != "v1" {
		t.Errorf("a.txt = %q, want v1 (working tree restored by recovery)", b)
	}
	_, runs, _ := rt.hist.List(ctx, sid)
	if len(runs) != 1 || runs[0].ID != "run1" {
		t.Errorf("runs after recovery = %+v, want only run1 (history truncated)", runs)
	}
	if pending, _ := rt.muts.ListPending(ctx); len(pending) != 0 {
		t.Errorf("pending after recovery = %+v, want none (intent cleared)", pending)
	}
}

// TestRecoverRollbacks_Idempotent: recovering an intent whose durable rollback
// already committed (history already truncated) is a no-op — the boundary
// recomputes empty and the restore is reentrant — and still clears the intent.
func TestRecoverRollbacks_Idempotent(t *testing.T) {
	s, rt, cp, sid, cwd := checkpointHarness(t)
	ctx := context.Background()

	writeFile(t, cwd, "a.txt", "v1")
	if err := cp.Snapshot(ctx, sid, cwd, "run1"); err != nil {
		t.Fatalf("snapshot run1: %v", err)
	}
	putRun(t, rt, sid, "run1", 1, 1)
	// Only run1 in history (run2 already dropped by the pre-crash rollback), tree
	// already at v1 — the "crashed after durable, before complete" state.
	if err := rt.muts.Record(ctx, execution.WorkspaceMutation{
		SessionID: sid, Cwd: cwd, ToRunID: "run1", RestoreHistory: true,
	}); err != nil {
		t.Fatalf("record intent: %v", err)
	}

	if err := s.RecoverRollbacks(ctx); err != nil {
		t.Fatalf("RecoverRollbacks: %v", err)
	}
	_, runs, _ := rt.hist.List(ctx, sid)
	if len(runs) != 1 || runs[0].ID != "run1" {
		t.Errorf("runs = %+v, want run1 untouched (idempotent no-op)", runs)
	}
	if pending, _ := rt.muts.ListPending(ctx); len(pending) != 0 {
		t.Errorf("pending after recovery = %+v, want cleared", pending)
	}
}

// TestRecoverRollbacks_FilesOnly restores an interrupted work-tree mutation but
// deliberately leaves the conversation timeline intact.
func TestRecoverRollbacks_FilesOnly(t *testing.T) {
	s, rt, cp, sid, cwd := checkpointHarness(t)
	ctx := context.Background()

	writeFile(t, cwd, "a.txt", "v1")
	if err := cp.Snapshot(ctx, sid, cwd, "run1"); err != nil {
		t.Fatalf("snapshot run1: %v", err)
	}
	putRun(t, rt, sid, "run1", 1, 1)
	writeFile(t, cwd, "a.txt", "v2")
	if err := cp.Snapshot(ctx, sid, cwd, "run2"); err != nil {
		t.Fatalf("snapshot run2: %v", err)
	}
	putRun(t, rt, sid, "run2", 2, 2)

	if err := rt.muts.Record(ctx, execution.WorkspaceMutation{
		SessionID: sid, Cwd: cwd, ToRunID: "run1", RestoreHistory: false,
	}); err != nil {
		t.Fatalf("record intent: %v", err)
	}
	if err := s.RecoverRollbacks(ctx); err != nil {
		t.Fatalf("RecoverRollbacks: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(cwd, "a.txt")); string(got) != "v1" {
		t.Fatalf("a.txt = %q, want v1", got)
	}
	_, runs, _ := rt.hist.List(ctx, sid)
	if len(runs) != 2 {
		t.Fatalf("files-only recovery dropped history: %+v", runs)
	}
	if pending, _ := rt.muts.ListPending(ctx); len(pending) != 0 {
		t.Fatalf("pending after files-only recovery = %+v, want none", pending)
	}
}

// TestRollback_FilesRequiresToRunID rejects a files restore with no target.
func TestRollback_FilesRequiresToRunID(t *testing.T) {
	s, _, _, sid, _ := checkpointHarness(t)
	_, err := s.RollbackSession(context.Background(), protocol.RollbackSessionRequest{
		SessionID: sid, RestoreType: protocol.RestoreFiles,
	})
	if !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("err = %v, want ErrInvalidParams", err)
	}
}

func TestRollbackRejectsUnknownRestoreType(t *testing.T) {
	s, rt := rollbackHarness(t)
	ctx := context.Background()
	ses, _ := rt.sess.Create(ctx, "t", t.TempDir())
	_, err := s.RollbackSession(ctx, protocol.RollbackSessionRequest{
		SessionID:   ses.ID,
		RestoreType: protocol.RestoreType("timeline"),
	})
	if !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("err = %v, want ErrInvalidParams", err)
	}
}

// TestRollback_NoCheckpointStore maps a files restore with no store onto
// checkpoint_unavailable (and leaves history untouched).
func TestRollback_NoCheckpointStore(t *testing.T) {
	s, rt := rollbackHarness(t)
	ctx := context.Background()
	ses, _ := rt.sess.Create(ctx, "t", t.TempDir())
	putRun(t, rt, ses.ID, "run1", 1, 1)
	putRun(t, rt, ses.ID, "run2", 2, 2)

	_, err := s.RollbackSession(ctx, protocol.RollbackSessionRequest{
		SessionID: ses.ID, ToRunID: "run1", RestoreType: protocol.RestoreBoth,
	})
	if !errors.Is(err, protocol.ErrCheckpointUnavailable) {
		t.Fatalf("err = %v, want ErrCheckpointUnavailable", err)
	}
	// Atomic "both": the files step failed first, so run2 must still be present.
	_, runs, _ := rt.hist.List(ctx, ses.ID)
	if len(runs) != 2 {
		t.Errorf("runs = %d, want 2 (history untouched after files failure)", len(runs))
	}
	if pending, _ := rt.muts.ListPending(ctx); len(pending) != 0 {
		t.Fatalf("unavailable checkpoint left unrecoverable intent: %+v", pending)
	}
}

type incompleteCheckpointRestorer struct{}

func (incompleteCheckpointRestorer) Restore(context.Context, string, string, string) error {
	return sessions.ErrCheckpointRestoreIncomplete
}

func (incompleteCheckpointRestorer) DropSession(string) error { return nil }

func TestRollback_IncompleteRestoreKeepsRecoveryIntent(t *testing.T) {
	s, rt := rollbackHarness(t)
	ctx := context.Background()
	ses, err := rt.sess.Create(ctx, "restore failure", t.TempDir())
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	putRun(t, rt, ses.ID, "run1", 1, 1)
	putRun(t, rt, ses.ID, "run2", 2, 2)
	s.sessions = rt.sessionsCoordinatorWithRestorer(incompleteCheckpointRestorer{})

	_, err = s.RollbackSession(ctx, protocol.RollbackSessionRequest{
		SessionID: ses.ID, ToRunID: "run1", RestoreType: protocol.RestoreBoth,
	})
	if !errors.Is(err, sessions.ErrCheckpointRestoreIncomplete) {
		t.Fatalf("rollback error = %v, want incomplete-restore sentinel", err)
	}
	pending, listErr := rt.muts.ListPending(ctx)
	if listErr != nil {
		t.Fatalf("list pending: %v", listErr)
	}
	if len(pending) != 1 || pending[0].SessionID != ses.ID || !pending[0].RestoreHistory {
		t.Fatalf("pending = %+v, want recoverable files+history intent", pending)
	}
	_, runs, _ := rt.hist.List(ctx, ses.ID)
	if len(runs) != 2 {
		t.Fatalf("incomplete file restore mutated history: %+v", runs)
	}
}
