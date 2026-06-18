package server

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/workspace"
)

// checkpointHarness extends the rollback harness with a real shadow-git
// checkpoint store and a session whose cwd is a populated temp dir. It returns
// the server, the session id, and the cwd so a test can mutate + snapshot.
func checkpointHarness(t *testing.T) (*Server, string, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed; skipping checkpoint rollback test")
	}
	s, rt := rollbackHarness(t)
	s.workspace = workspace.New(t.TempDir())
	cwd := t.TempDir()
	// Checkpoints only fire in a real git repo now (workspace.Snapshot's gate,
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
	return s, ses.ID, cwd
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
	s, sid, cwd := checkpointHarness(t)
	ctx := context.Background()
	rt := s.rt.(*stubRuntime)

	writeFile(t, cwd, "a.txt", "v1")
	if err := s.workspace.Snapshot(ctx, sid, cwd, "run1"); err != nil {
		t.Fatalf("snapshot run1: %v", err)
	}
	putRun(t, rt, sid, "run1", "", 1, 1)

	writeFile(t, cwd, "a.txt", "v2")
	if err := s.workspace.Snapshot(ctx, sid, cwd, "run2"); err != nil {
		t.Fatalf("snapshot run2: %v", err)
	}
	putRun(t, rt, sid, "run2", "", 2, 2)

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
	s, sid, cwd := checkpointHarness(t)
	ctx := context.Background()
	rt := s.rt.(*stubRuntime)

	writeFile(t, cwd, "a.txt", "v1")
	s.workspace.Snapshot(ctx, sid, cwd, "run1")
	putRun(t, rt, sid, "run1", "", 1, 1)
	writeFile(t, cwd, "a.txt", "v2")
	s.workspace.Snapshot(ctx, sid, cwd, "run2")
	putRun(t, rt, sid, "run2", "", 2, 2)

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
}

// TestRollback_FilesRequiresToRunID rejects a files restore with no target.
func TestRollback_FilesRequiresToRunID(t *testing.T) {
	s, sid, _ := checkpointHarness(t)
	_, err := s.RollbackSession(context.Background(), protocol.RollbackSessionRequest{
		SessionID: sid, RestoreType: protocol.RestoreFiles,
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
	putRun(t, rt, ses.ID, "run1", "", 1, 1)
	putRun(t, rt, ses.ID, "run2", "", 2, 2)

	_, err := s.RollbackSession(ctx, protocol.RollbackSessionRequest{
		SessionID: ses.ID, ToRunID: "run1", RestoreType: protocol.RestoreBoth,
	})
	if !errors.Is(err, protocol.ErrCheckpointUnavailable) {
		t.Fatalf("err = %v, want ErrCheckpointUnavailable", err)
	}
	// Atomic "both": the files step failed first, so run2 must still be present.
	_, runs, _ := s.rt.Transcript().List(ctx, ses.ID)
	if len(runs) != 2 {
		t.Errorf("runs = %d, want 2 (history untouched after files failure)", len(runs))
	}
}
