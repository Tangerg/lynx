package git

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestRepositoryReadsPreserveCancellation(t *testing.T) {
	dir := initRepo(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	reads := []struct {
		name string
		read func() error
	}{
		{name: "changes", read: func() error { _, err := ListChanges(ctx, dir); return err }},
		{name: "files", read: func() error { _, err := ListFiles(ctx, dir, "."); return err }},
		{name: "structured diff", read: func() error { _, err := Diff(ctx, dir, "", Worktree); return err }},
		{name: "raw diff", read: func() error { _, err := RawDiff(ctx, dir, "", Worktree); return err }},
	}
	for _, test := range reads {
		t.Run(test.name, func(t *testing.T) {
			if err := test.read(); !errors.Is(err, context.Canceled) {
				t.Fatalf("repository read error = %v, want context.Canceled", err)
			}
		})
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

func TestNestedWorkspaceVCSReadsStayJailedAndWorkspaceRelative(t *testing.T) {
	root := initRepo(t)
	nested := filepath.Join(root, "packages", "desktop")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested workspace: %v", err)
	}
	write(t, nested, "inside.txt", "before\n")
	gitTestCommand(t, root, "add", "packages/desktop/inside.txt")
	gitTestCommand(t, root, "commit", "-m", "add nested workspace")

	write(t, root, "a.txt", "outside change\n")
	write(t, nested, "inside.txt", "after\n")
	write(t, root, "outside-untracked.txt", "outside\n")
	write(t, nested, "inside-untracked.txt", "one\ntwo\n")

	changes, err := ListChanges(t.Context(), nested)
	if err != nil {
		t.Fatalf("ListChanges: %v", err)
	}
	if len(changes) != 2 {
		t.Fatalf("changes = %+v, want only the two nested workspace files", changes)
	}
	byPath := make(map[string]FileChange, len(changes))
	for _, change := range changes {
		byPath[change.Path] = change
	}
	if change := byPath["inside.txt"]; change.Status != StatusModified {
		t.Fatalf("inside.txt = %+v, want modified", change)
	}
	if change := byPath["inside-untracked.txt"]; change.Status != StatusUntracked || change.Added != 2 {
		t.Fatalf("inside-untracked.txt = %+v, want untracked with two added lines", change)
	}
	for _, outside := range []string{"a.txt", "outside-untracked.txt", "packages/desktop/inside.txt"} {
		if _, leaked := byPath[outside]; leaked {
			t.Fatalf("outside or repository-relative path %q leaked into nested workspace", outside)
		}
	}

	files, err := Diff(t.Context(), nested, "", Worktree)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	diffByPath := make(map[string]DiffFile, len(files))
	for _, file := range files {
		diffByPath[file.Path] = file
	}
	if len(files) != 2 || diffByPath["inside.txt"].Status != StatusModified || diffByPath["inside-untracked.txt"].Status != StatusUntracked {
		t.Fatalf("diff files = %+v, want workspace-relative nested files", files)
	}
	selected, err := Diff(t.Context(), nested, filepath.Join(nested, "inside.txt"), Worktree)
	if err != nil {
		t.Fatalf("Diff selected absolute path: %v", err)
	}
	if len(selected) != 1 || selected[0].Path != "inside.txt" {
		t.Fatalf("selected diff = %+v, want inside.txt", selected)
	}
	if _, err := Diff(t.Context(), nested, filepath.Join(root, "a.txt"), Worktree); err == nil {
		t.Fatal("Diff accepted a path outside the nested workspace")
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

func TestRunPreservesContextCancellation(t *testing.T) {
	if !Available() {
		t.Skip("git not on PATH")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := run(ctx, t.TempDir(), "status", "--short")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("run error = %v, want context.Canceled", err)
	}
}

func TestRunAllowsDocumentedExitCode(t *testing.T) {
	if !Available() {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	write(t, dir, "new.txt", "new\n")
	out, err := runAllowingExitCode(t.Context(), dir, 1, "diff", "--no-index", "--", os.DevNull, "new.txt")
	if err != nil {
		t.Fatalf("runAllowingExitCode: %v", err)
	}
	if !strings.Contains(out, "+new") {
		t.Fatalf("diff output = %q, want added content", out)
	}
}

func TestStatusObservationDoesNotRefreshGitIndex(t *testing.T) {
	dir := initRepo(t)
	indexPath := filepath.Join(dir, ".git", "index")
	before, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read index before ListChanges: %v", err)
	}

	// Change only the worktree stat tuple. An ordinary `git status` proves the
	// content is unchanged and then refreshes the cached tuple in the index;
	// --no-optional-locks must keep this observation from writing that refresh.
	info, err := os.Stat(filepath.Join(dir, "a.txt"))
	if err != nil {
		t.Fatalf("stat tracked file: %v", err)
	}
	changed := info.ModTime().Add(2 * time.Second)
	if err := os.Chtimes(filepath.Join(dir, "a.txt"), changed, changed); err != nil {
		t.Fatalf("change tracked file timestamp: %v", err)
	}

	status, err := run(t.Context(), dir, "status", "--porcelain=v1", "-z")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status != "" {
		t.Fatalf("timestamp-only change reported as content change: %q", status)
	}
	after, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read index after ListChanges: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("read-only status refreshed .git/index")
	}
}

func TestListChangesIgnoresAmbientRepositoryRouting(t *testing.T) {
	dir := initRepo(t)
	foreign := initRepo(t)
	write(t, dir, "a.txt", "changed in requested workspace\n")

	// Git gives these variables precedence over -C. A Runtime embedded in a
	// parent Git process must still observe the explicitly requested workspace.
	t.Setenv("GIT_DIR", filepath.Join(foreign, ".git"))
	t.Setenv("GIT_WORK_TREE", foreign)

	changes, err := ListChanges(t.Context(), dir)
	if err != nil {
		t.Fatalf("ListChanges with ambient repository routing: %v", err)
	}
	if len(changes) != 1 || changes[0].Path != "a.txt" || changes[0].Status != StatusModified {
		t.Fatalf("changes = %+v, want requested workspace's modified a.txt", changes)
	}
}

func TestParseUnifiedDiffRejectsMalformedHunkHeader(t *testing.T) {
	patch := "diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -invalid +1 @@\n-old\n+new\n"
	if _, err := parseUnifiedDiff(patch); err == nil {
		t.Fatal("parseUnifiedDiff accepted a malformed hunk header")
	}
}

func TestApplyNumstatRejectsMalformedCounts(t *testing.T) {
	changes := map[string]*FileChange{"a.txt": {Path: "a.txt"}}
	if err := applyNumstatZ("invalid\t1\ta.txt\x00", changes); err == nil {
		t.Fatal("applyNumstatZ accepted a malformed added-line count")
	}
}

func gitTestCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}
