package checkpoint

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed; skipping checkpoint test")
	}
	return NewStore(t.TempDir()), t.TempDir() // (shadow root, work tree)
}

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func read(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return ""
	}
	return string(b)
}

// TestStore_SnapshotRestore exercises the round trip: snapshot run1, mutate +
// add a file, snapshot run2, then restore run1 — the tracked file reverts and
// the file added after run1 is removed.
func TestStore_SnapshotRestore(t *testing.T) {
	s, cwd := newTestStore(t)
	ctx := context.Background()

	write(t, cwd, "a.txt", "v1")
	if err := s.Snapshot(ctx, "ses1", cwd, "run1"); err != nil {
		t.Fatalf("snapshot run1: %v", err)
	}

	write(t, cwd, "a.txt", "v2")
	write(t, cwd, "b.txt", "new")
	if err := s.Snapshot(ctx, "ses1", cwd, "run2"); err != nil {
		t.Fatalf("snapshot run2: %v", err)
	}

	if err := s.Restore(ctx, "ses1", cwd, "run1"); err != nil {
		t.Fatalf("restore run1: %v", err)
	}
	if got := read(t, cwd, "a.txt"); got != "v1" {
		t.Errorf("a.txt = %q, want v1 (reverted)", got)
	}
	if _, err := os.Stat(filepath.Join(cwd, "b.txt")); !os.IsNotExist(err) {
		t.Error("b.txt should be removed (added after run1)")
	}
}

// TestStore_RestoreUnknownRun reports ErrUnavailable for a run never snapshotted.
func TestStore_RestoreUnknownRun(t *testing.T) {
	s, cwd := newTestStore(t)
	ctx := context.Background()
	write(t, cwd, "a.txt", "v1")
	if err := s.Snapshot(ctx, "ses1", cwd, "run1"); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if err := s.Restore(ctx, "ses1", cwd, "ghost"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("restore ghost = %v, want ErrUnavailable", err)
	}
}

// TestStore_HonorsGitignore: a file matched by the work-tree .gitignore is not
// captured, so restoring doesn't delete or revert it.
func TestStore_HonorsGitignore(t *testing.T) {
	s, cwd := newTestStore(t)
	ctx := context.Background()
	write(t, cwd, ".gitignore", "ignored.txt\n")
	write(t, cwd, "tracked.txt", "v1")
	if err := s.Snapshot(ctx, "ses1", cwd, "run1"); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	// Create an ignored file after the snapshot; restore must leave it alone.
	write(t, cwd, "ignored.txt", "keep me")
	write(t, cwd, "tracked.txt", "v2")
	if err := s.Restore(ctx, "ses1", cwd, "run1"); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if got := read(t, cwd, "tracked.txt"); got != "v1" {
		t.Errorf("tracked.txt = %q, want v1", got)
	}
	if read(t, cwd, "ignored.txt") != "keep me" {
		t.Error("ignored.txt should survive restore (gitignored, never snapshotted)")
	}
}
