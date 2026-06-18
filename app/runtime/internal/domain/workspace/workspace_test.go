package workspace_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/git"
)

// TestSnapshot_SkipsNonGitDir is the regression guard for the run-teardown hang:
// a session opened on a non-git directory (e.g. the home dir) must NOT be
// snapshotted — a whole-tree `git add` there would stage the entire tree and
// block. The gate (workspace.Snapshot → git.IsRepo) makes it a silent no-op, so
// no shadow repo is ever created.
func TestSnapshot_SkipsNonGitDir(t *testing.T) {
	if !git.Available() {
		t.Skip("git not installed")
	}
	root := t.TempDir() // shadow-repo store root
	cwd := t.TempDir()  // a plain dir — NOT a git repo
	svc := workspace.New(root)

	if err := svc.Snapshot(context.Background(), "ses1", cwd, "run1"); err != nil {
		t.Fatalf("Snapshot on a non-git dir should no-op, got: %v", err)
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatalf("non-git dir was snapshotted (shadow repo created): %v", entries)
	}
}

// TestSnapshot_RunsInGitRepo confirms the gate still lets a real repo through:
// the shadow repo is created and the boundary is anchored.
func TestSnapshot_RunsInGitRepo(t *testing.T) {
	if !git.Available() {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	cwd := t.TempDir()
	if out, err := exec.Command("git", "-C", cwd, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	if err := os.WriteFile(filepath.Join(cwd, "a.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := workspace.New(root)

	if err := svc.Snapshot(context.Background(), "ses1", cwd, "run1"); err != nil {
		t.Fatalf("Snapshot in a git repo: %v", err)
	}
	if entries, _ := os.ReadDir(root); len(entries) == 0 {
		t.Fatal("git repo was not snapshotted (no shadow repo created)")
	}
}
