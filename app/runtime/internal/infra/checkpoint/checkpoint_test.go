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

func TestStore_StopsTrackingFileThatGrowsPastLimit(t *testing.T) {
	s, cwd := newTestStore(t)
	ctx := context.Background()

	write(t, cwd, "changing.bin", "small")
	if err := s.Snapshot(ctx, "ses1", cwd, "run1"); err != nil {
		t.Fatalf("snapshot run1: %v", err)
	}
	write(t, cwd, "changing.bin", strings.Repeat("A", maxCheckpointFileSize+1))
	if err := s.Snapshot(ctx, "ses1", cwd, "run2"); err != nil {
		t.Fatalf("snapshot run2: %v", err)
	}
	if _, err := s.git(ctx, s.gitDir("ses1"), cwd, "cat-file", "-e", tagFor("run2")+":changing.bin"); err == nil {
		t.Fatal("run2 still owns file after it crossed the size cap")
	}

	write(t, cwd, "changing.bin", strings.Repeat("B", maxCheckpointFileSize+1))
	if err := s.Restore(ctx, "ses1", cwd, "run2"); err != nil {
		t.Fatalf("restore run2: %v", err)
	}
	if got := read(t, cwd, "changing.bin"); !strings.HasPrefix(got, "B") {
		t.Fatalf("oversized untracked file was changed by restore; got prefix %.1q", got)
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

// TestStore_MaterializesSourceRepoSeed: a source index avoids re-hashing the
// baseline, but the completed checkpoint owns every reachable object and still
// restores after the source object database disappears.
func TestStore_MaterializesSourceRepoSeed(t *testing.T) {
	s, cwd := newTestStore(t)
	ctx := context.Background()

	gitCmd(t, cwd, "init", "-q")
	write(t, cwd, "committed.txt", "hello")
	gitCmd(t, cwd, "add", ".")
	gitCmd(t, cwd, "commit", "-qm", "init")

	if err := s.Snapshot(ctx, "ses1", cwd, "run1"); err != nil {
		t.Fatalf("snapshot run1: %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.gitDir("ses1"), "objects", "info", "alternates")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("completed checkpoint still depends on source alternates: %v", err)
	}

	// Take the real object DB offline before the round trip. A shadow repository
	// that still borrowed any baseline tree or blob would now fail to restore.
	sourceObjects := filepath.Join(cwd, ".git", "objects")
	if err := os.Rename(sourceObjects, sourceObjects+".offline"); err != nil {
		t.Fatalf("take source object store offline: %v", err)
	}
	write(t, cwd, "committed.txt", "world")
	if err := s.Snapshot(ctx, "ses1", cwd, "run2"); err != nil {
		t.Fatalf("snapshot run2: %v", err)
	}
	if err := s.Restore(ctx, "ses1", cwd, "run1"); err != nil {
		t.Fatalf("restore run1: %v", err)
	}
	if got := read(t, cwd, "committed.txt"); got != "hello" {
		t.Errorf("committed.txt = %q, want hello (restored from self-contained shadow)", got)
	}
}

func TestStore_AppliesSizeLimitToSourceIndex(t *testing.T) {
	s, cwd := newTestStore(t)
	ctx := context.Background()

	gitCmd(t, cwd, "init", "-q")
	write(t, cwd, "small.txt", "small")
	write(t, cwd, "tracked-large.bin", strings.Repeat("A", maxCheckpointFileSize+1))
	gitCmd(t, cwd, "add", ".")
	gitCmd(t, cwd, "commit", "-qm", "init")

	if err := s.Snapshot(ctx, "ses1", cwd, "run1"); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if _, err := s.git(ctx, s.gitDir("ses1"), cwd, "cat-file", "-e", tagFor("run1")+":tracked-large.bin"); err == nil {
		t.Fatal("baseline snapshot retained oversized file copied from source index")
	}
	if _, err := s.git(ctx, s.gitDir("ses1"), cwd, "cat-file", "-e", tagFor("run1")+":small.txt"); err != nil {
		t.Fatalf("baseline snapshot lost small source-index file: %v", err)
	}

	write(t, cwd, "tracked-large.bin", strings.Repeat("B", maxCheckpointFileSize+1))
	if err := s.Restore(ctx, "ses1", cwd, "run1"); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if got := read(t, cwd, "tracked-large.bin"); !strings.HasPrefix(got, "B") {
		t.Fatalf("oversized source-index file was changed by restore; got prefix %.1q", got)
	}
}

// TestStore_FailedSeedDoesNotPublishRepository verifies that initialization is
// transactional. A malformed source index fails after the staging repo has
// already been initialized, but no HEAD or partial seed becomes visible at the
// session's final path; fixing the source allows the next attempt to start
// cleanly and complete.
func TestStore_FailedSeedDoesNotPublishRepository(t *testing.T) {
	s, cwd := newTestStore(t)
	ctx := context.Background()

	gitCmd(t, cwd, "init", "-q")
	sourceIndex := filepath.Join(cwd, ".git", "index")
	if err := os.Mkdir(sourceIndex, 0o755); err != nil {
		t.Fatalf("create malformed source index: %v", err)
	}
	if _, err := s.ensureRepo(ctx, "ses1", cwd); err == nil || !strings.Contains(err.Error(), "source index") {
		t.Fatalf("ensure repo error = %v, want source-index error", err)
	}
	if _, err := os.Stat(s.gitDir("ses1")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed initialization published repository: %v", err)
	}
	entries, err := os.ReadDir(s.root)
	if err != nil {
		t.Fatalf("read checkpoint root: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("failed initialization left staging entries: %v", entries)
	}

	if err := os.Remove(sourceIndex); err != nil {
		t.Fatalf("remove malformed source index: %v", err)
	}
	write(t, cwd, "a.txt", "v1")
	if err := s.Snapshot(ctx, "ses1", cwd, "run1"); err != nil {
		t.Fatalf("snapshot after repairing source repo: %v", err)
	}
	if !repoExists(s.gitDir("ses1")) {
		t.Fatal("successful retry did not publish repository")
	}
}

func TestStore_ResolvesRelativeSourceAlternates(t *testing.T) {
	s, cwd := newTestStore(t)
	ctx := context.Background()

	gitCmd(t, cwd, "init", "-q")
	sourceObjects := filepath.Join(cwd, ".git", "objects")
	borrowedObjects := filepath.Join(cwd, ".git", "borrowed-objects")
	if err := os.MkdirAll(borrowedObjects, 0o755); err != nil {
		t.Fatalf("create borrowed object store: %v", err)
	}
	write(t, filepath.Join(sourceObjects, "info"), "alternates", "../borrowed-objects\n")

	gitDir, err := s.ensureRepo(ctx, "ses1", cwd)
	if err != nil {
		t.Fatalf("ensure repo: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(gitDir, "objects", "info", "alternates"))
	if err != nil {
		t.Fatalf("read shadow alternates: %v", err)
	}
	if !strings.Contains(string(data), borrowedObjects) {
		t.Fatalf("shadow alternates %q do not contain resolved path %q", data, borrowedObjects)
	}
}

func TestStore_SeedsFromLinkedWorktreeIndex(t *testing.T) {
	s, mainWorktree := newTestStore(t)
	ctx := context.Background()

	gitCmd(t, mainWorktree, "init", "-q")
	write(t, mainWorktree, "tracked.txt", "main")
	gitCmd(t, mainWorktree, "add", ".")
	gitCmd(t, mainWorktree, "commit", "-qm", "init")

	linkedWorktree := filepath.Join(t.TempDir(), "linked")
	gitCmd(t, mainWorktree, "worktree", "add", "-q", "-b", "linked", linkedWorktree)
	write(t, linkedWorktree, "tracked.txt", "linked")
	if err := s.Snapshot(ctx, "ses1", linkedWorktree, "run1"); err != nil {
		t.Fatalf("snapshot linked worktree: %v", err)
	}
	write(t, linkedWorktree, "tracked.txt", "after")
	if err := s.Restore(ctx, "ses1", linkedWorktree, "run1"); err != nil {
		t.Fatalf("restore linked worktree: %v", err)
	}
	if got := read(t, linkedWorktree, "tracked.txt"); got != "linked" {
		t.Fatalf("tracked.txt = %q, want linked-worktree snapshot", got)
	}
}

func TestStore_DoesNotReplaceRepositoryOnInspectionFailure(t *testing.T) {
	s, cwd := newTestStore(t)
	ctx := context.Background()

	write(t, cwd, "tracked.txt", "v1")
	if err := s.Snapshot(ctx, "ses1", cwd, "run1"); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	gitDir := s.gitDir("ses1")
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("invalid ref\n"), 0o644); err != nil {
		t.Fatalf("corrupt HEAD: %v", err)
	}
	if err := s.Snapshot(ctx, "ses1", cwd, "run2"); err == nil {
		t.Fatal("snapshot replaced a repository whose HEAD inspection failed")
	}
	if _, err := os.Stat(filepath.Join(gitDir, "refs", "tags", tagFor("run1"))); err != nil {
		t.Fatalf("inspection failure destroyed existing snapshot refs: %v", err)
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
