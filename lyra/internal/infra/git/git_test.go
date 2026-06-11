package git

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// initRepo creates a temp git repo with one committed file ("a.txt") and
// returns its dir. Skips the test if git isn't installed.
func initRepo(t *testing.T) string {
	t.Helper()
	if !Available() {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	env := append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	gitCmd := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir, cmd.Env = dir, env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	gitCmd("init", "-b", "main")
	write(t, dir, "a.txt", "a\nb\nc\n")
	gitCmd("add", ".")
	gitCmd("commit", "-m", "init")
	return dir
}

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestListChangesAndDiff: a modified tracked file + an untracked file show up
// in both ListChanges (with line counts) and Diff (worktree, with rows).
func TestListChangesAndDiff(t *testing.T) {
	dir := initRepo(t)
	ctx := context.Background()
	write(t, dir, "a.txt", "a\nB\nc\nd\n") // modify line 2, add line 4
	write(t, dir, "new.txt", "x\ny\n")     // untracked

	changes, err := ListChanges(ctx, dir)
	if err != nil {
		t.Fatalf("ListChanges: %v", err)
	}
	byPath := map[string]FileChange{}
	for _, c := range changes {
		byPath[c.Path] = c
	}
	if c := byPath["a.txt"]; c.Status != StatusModified || c.Added == 0 {
		t.Errorf("a.txt = %+v, want modified with added>0", c)
	}
	if c := byPath["new.txt"]; c.Status != StatusUntracked {
		t.Errorf("new.txt status = %q, want untracked", c.Status)
	}

	files, err := Diff(ctx, dir, "", Worktree)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	dByPath := map[string]DiffFile{}
	for _, f := range files {
		dByPath[f.Path] = f
	}
	if f, ok := dByPath["a.txt"]; !ok || len(f.Rows) == 0 || f.Added == 0 {
		t.Errorf("a.txt diff = %+v, want rows + added>0", f)
	}
	// untracked file appears as an all-added diff
	if f, ok := dByPath["new.txt"]; !ok || f.Status != StatusUntracked || f.Added != 2 {
		t.Errorf("new.txt diff = %+v, want untracked + added=2", f)
	}
}

// TestDiffRowsStructure: rows carry the right type + line numbers.
func TestDiffRowsStructure(t *testing.T) {
	dir := initRepo(t)
	write(t, dir, "a.txt", "a\nB\nc\n") // change line 2: b → B
	files, err := Diff(context.Background(), dir, "a.txt", Worktree)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("files = %d, want 1", len(files))
	}
	var added, deleted, hunk int
	for _, r := range files[0].Rows {
		switch r.Type {
		case "added":
			added++
		case "deleted":
			deleted++
		case "hunk":
			hunk++
		}
	}
	if hunk == 0 || added == 0 || deleted == 0 {
		t.Errorf("rows missing kinds: hunk=%d added=%d deleted=%d", hunk, added, deleted)
	}
}

// TestNotRepo: a plain temp dir (no git init) reports ErrNotRepo.
func TestNotRepo(t *testing.T) {
	if !Available() {
		t.Skip("git not on PATH")
	}
	if _, err := ListChanges(context.Background(), t.TempDir()); !errors.Is(err, ErrNotRepo) {
		t.Errorf("ListChanges on non-repo err = %v, want ErrNotRepo", err)
	}
}
