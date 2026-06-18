package checkpoint

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// TestStore_SkipsLargeFiles: a file over maxCheckpointFileSize is left out of
// the snapshot, so restore neither reverts it (it's not tracked) nor deletes it
// (no `git clean`). A small sibling still round-trips normally.
func TestStore_SkipsLargeFiles(t *testing.T) {
	s, cwd := newTestStore(t)
	ctx := context.Background()

	write(t, cwd, "small.txt", "v1")
	write(t, cwd, "big.bin", strings.Repeat("A", maxCheckpointFileSize+1024))
	if err := s.Snapshot(ctx, "ses1", cwd, "run1"); err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	write(t, cwd, "small.txt", "v2")
	write(t, cwd, "big.bin", strings.Repeat("B", maxCheckpointFileSize+1024))
	if err := s.Restore(ctx, "ses1", cwd, "run1"); err != nil {
		t.Fatalf("restore: %v", err)
	}

	if got := read(t, cwd, "small.txt"); got != "v1" {
		t.Errorf("small.txt = %q, want v1 (tracked → reverted)", got)
	}
	// The oversize file was never snapshotted: it must survive the restore in
	// its current (post-snapshot) state — neither reverted nor deleted.
	if got := read(t, cwd, "big.bin"); !strings.HasPrefix(got, "B") {
		t.Errorf("big.bin should be untouched (over size cap, never tracked); got prefix %.1q", got)
	}
}

// gitCmd runs a real git command in dir (the source repo a seed test builds),
// independent of the user's global config.
func gitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

// TestStore_NoChangeRunReusesHeadCommit: a run that changed nothing must NOT
// mint an empty commit — it re-tags the existing HEAD, so the commit count is
// unchanged and both run tags resolve to the same commit. Restore still works.
func TestStore_NoChangeRunReusesHeadCommit(t *testing.T) {
	s, cwd := newTestStore(t)
	ctx := context.Background()
	gitDir := s.gitDir("ses1")

	write(t, cwd, "a.txt", "v1")
	if err := s.Snapshot(ctx, "ses1", cwd, "run1"); err != nil {
		t.Fatalf("snapshot run1: %v", err)
	}
	before, _ := s.git(ctx, gitDir, cwd, "rev-list", "--count", "HEAD")

	// run2 changes nothing.
	if err := s.Snapshot(ctx, "ses1", cwd, "run2"); err != nil {
		t.Fatalf("snapshot run2: %v", err)
	}
	after, _ := s.git(ctx, gitDir, cwd, "rev-list", "--count", "HEAD")
	if before != after {
		t.Errorf("commit count %s→%s: a no-change run must not mint an empty commit", before, after)
	}
	c1, _ := s.git(ctx, gitDir, cwd, "rev-parse", tagFor("run1"))
	c2, _ := s.git(ctx, gitDir, cwd, "rev-parse", tagFor("run2"))
	if c1 != c2 {
		t.Errorf("run1=%s run2=%s: no-change run2 should re-tag run1's commit", c1, c2)
	}
	if err := s.Restore(ctx, "ses1", cwd, "run2"); err != nil {
		t.Fatalf("restore run2: %v", err)
	}
	if got := read(t, cwd, "a.txt"); got != "v1" {
		t.Errorf("a.txt = %q, want v1", got)
	}
}

// TestStore_SeedsFromSourceRepo: when cwd is a real git repo, a fresh shadow
// repo shares its object store via objects/info/alternates (so the baseline
// isn't duplicated) and a round-trip restore still works through the shared
// objects.
func TestStore_SeedsFromSourceRepo(t *testing.T) {
	s, cwd := newTestStore(t)
	ctx := context.Background()

	gitCmd(t, cwd, "init", "-q")
	write(t, cwd, "committed.txt", "hello")
	gitCmd(t, cwd, "add", ".")
	gitCmd(t, cwd, "commit", "-qm", "init")

	if err := s.Snapshot(ctx, "ses1", cwd, "run1"); err != nil {
		t.Fatalf("snapshot run1: %v", err)
	}
	alt, err := os.ReadFile(filepath.Join(s.gitDir("ses1"), "objects", "info", "alternates"))
	if err != nil {
		t.Fatalf("shadow repo should seed objects/info/alternates from the real repo: %v", err)
	}
	if !strings.Contains(string(alt), "objects") {
		t.Errorf("alternates %q should point at an object store", alt)
	}

	// Round-trip through the shared object DB: edit, snapshot, restore the first.
	write(t, cwd, "committed.txt", "world")
	if err := s.Snapshot(ctx, "ses1", cwd, "run2"); err != nil {
		t.Fatalf("snapshot run2: %v", err)
	}
	if err := s.Restore(ctx, "ses1", cwd, "run1"); err != nil {
		t.Fatalf("restore run1: %v", err)
	}
	if got := read(t, cwd, "committed.txt"); got != "hello" {
		t.Errorf("committed.txt = %q, want hello (restored via shared object)", got)
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
