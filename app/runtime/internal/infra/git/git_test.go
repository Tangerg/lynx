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

func testChanges(ctx context.Context, dir string) ([]FileChange, error) {
	return ListChanges(ctx, dir, 10_000)
}

const testMaxDiffBytes = 64 << 20

func testDiff(ctx context.Context, dir, path string, mode Mode) ([]DiffFile, error) {
	files, _, err := Diff(ctx, dir, path, mode, 5_000, 5_000, testMaxDiffBytes)
	return files, err
}

func TestRepositoryReadsPreserveCancellation(t *testing.T) {
	dir := initRepo(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	reads := []struct {
		name string
		read func() error
	}{
		{name: "changes", read: func() error { _, err := testChanges(ctx, dir); return err }},
		{name: "files", read: func() error { _, err := ListFiles(ctx, dir, ".", 20_000); return err }},
		{name: "structured diff", read: func() error { _, err := testDiff(ctx, dir, "", Worktree); return err }},
		{name: "raw diff", read: func() error { _, err := RawDiff(ctx, dir, "", Worktree, testMaxDiffBytes); return err }},
	}
	for _, test := range reads {
		t.Run(test.name, func(t *testing.T) {
			if err := test.read(); !errors.Is(err, context.Canceled) {
				t.Fatalf("repository read error = %v, want context.Canceled", err)
			}
		})
	}
}

func TestWorktreeDiffKeepsUnbornRepositorySemantics(t *testing.T) {
	if !Available() {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	gitTestCommand(t, dir, "init", "-b", "main")
	write(t, dir, "new.txt", "new\n")

	files, err := testDiff(t.Context(), dir, "", Worktree)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(files) != 1 || files[0].Path != "new.txt" || files[0].Status != StatusUntracked {
		t.Fatalf("Diff = %+v, want unborn repository's untracked file", files)
	}
	patch, err := RawDiff(t.Context(), dir, "", Worktree, testMaxDiffBytes)
	if err != nil {
		t.Fatalf("RawDiff: %v", err)
	}
	if !strings.Contains(patch, "new.txt") {
		t.Fatalf("RawDiff = %q, want new.txt", patch)
	}
}

// TestListChangesAndDiff: a modified tracked file + an untracked file show up
// in both ListChanges (with line counts) and Diff (worktree, with rows).
func TestListChangesAndDiff(t *testing.T) {
	dir := initRepo(t)
	ctx := context.Background()
	write(t, dir, "a.txt", "a\nB\nc\nd\n") // modify line 2, add line 4
	write(t, dir, "new.txt", "x\ny\n")     // untracked

	changes, err := testChanges(ctx, dir)
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

	files, err := testDiff(ctx, dir, "", Worktree)
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

	changes, err := testChanges(t.Context(), nested)
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

	files, err := testDiff(t.Context(), nested, "", Worktree)
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
	selected, err := testDiff(t.Context(), nested, filepath.Join(nested, "inside.txt"), Worktree)
	if err != nil {
		t.Fatalf("Diff selected absolute path: %v", err)
	}
	if len(selected) != 1 || selected[0].Path != "inside.txt" {
		t.Fatalf("selected diff = %+v, want inside.txt", selected)
	}
	if _, err := testDiff(t.Context(), nested, filepath.Join(root, "a.txt"), Worktree); err == nil {
		t.Fatal("Diff accepted a path outside the nested workspace")
	}
}

// TestDiffRowsStructure: rows carry the right type + line numbers.
func TestDiffRowsStructure(t *testing.T) {
	dir := initRepo(t)
	write(t, dir, "a.txt", "a\nB\nc\n") // change line 2: b → B
	files, err := testDiff(context.Background(), dir, "a.txt", Worktree)
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
	if _, err := testChanges(context.Background(), t.TempDir()); !errors.Is(err, ErrNotRepo) {
		t.Errorf("ListChanges on non-repo err = %v, want ErrNotRepo", err)
	}
}

func TestRepositoryProbeDoesNotConflateExit128FailuresWithNotRepo(t *testing.T) {
	for _, diagnostic := range []string{
		"fatal: detected dubious ownership in repository at '/workspace'",
		"fatal: bad object HEAD",
		"fatal: unable to read current working directory: Permission denied",
	} {
		if isNotRepositoryDiagnostic(diagnostic) {
			t.Fatalf("diagnostic %q was classified as a non-repository", diagnostic)
		}
	}
	if !isNotRepositoryDiagnostic("fatal: not a git repository (or any of the parent directories): .git") {
		t.Fatal("canonical non-repository diagnostic was not classified")
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
	if !bytes.Contains(out, []byte("+new")) {
		t.Fatalf("diff output = %q, want added content", out)
	}
}

func TestRunRejectsUnboundedGitOutput(t *testing.T) {
	dir := initRepo(t)
	const maxOutput = 64 << 20
	path := filepath.Join(dir, "large.txt")
	if err := os.WriteFile(path, bytes.Repeat([]byte{'x'}, maxOutput+1), 0o644); err != nil {
		t.Fatal(err)
	}
	hash := strings.TrimSpace(gitTestCommandOutput(t, dir, "hash-object", "-w", path))

	if _, err := run(t.Context(), dir, "cat-file", "blob", hash); err == nil {
		t.Fatal("run accepted Git stdout larger than 64 MiB")
	}
}

func TestListChangesUsesGitSymlinkStatInsteadOfReadingTheTarget(t *testing.T) {
	dir := initRepo(t)
	targetDir := t.TempDir()
	write(t, targetDir, "outside.txt", "one\ntwo\nthree\n")
	if err := os.Symlink(filepath.Join(targetDir, "outside.txt"), filepath.Join(dir, "link.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	changes, err := testChanges(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range changes {
		if change.Path == "link.txt" {
			if change.Added != 1 || change.Binary {
				t.Fatalf("symlink stat = added:%d binary:%v, want one Git symlink line", change.Added, change.Binary)
			}
			return
		}
	}
	t.Fatal("untracked symlink was absent from Changes")
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
	if chtimesErr := os.Chtimes(filepath.Join(dir, "a.txt"), changed, changed); chtimesErr != nil {
		t.Fatalf("change tracked file timestamp: %v", chtimesErr)
	}

	status, err := run(t.Context(), dir, "status", "--porcelain=v1", "-z")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if len(status) != 0 {
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

	changes, err := testChanges(t.Context(), dir)
	if err != nil {
		t.Fatalf("ListChanges with ambient repository routing: %v", err)
	}
	if len(changes) != 1 || changes[0].Path != "a.txt" || changes[0].Status != StatusModified {
		t.Fatalf("changes = %+v, want requested workspace's modified a.txt", changes)
	}
}

func TestParseUnifiedDiffRejectsMalformedHunkHeader(t *testing.T) {
	patch := "diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -invalid +1 @@\n-old\n+new\n"
	if _, _, err := parseUnifiedDiff([]byte(patch), 5_000, 5_000); err == nil {
		t.Fatal("parseUnifiedDiff accepted a malformed hunk header")
	}
}

func TestParseUnifiedDiffStopsBeforeAnOversizedWholeFile(t *testing.T) {
	patch := []byte("diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -0,0 +1 @@\n+line\n")
	files, truncated, err := parseUnifiedDiff(patch, 5, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 || !truncated {
		t.Fatalf("parse = %d files, truncated=%v; want a whole-file cut", len(files), truncated)
	}
}

func TestParseUnifiedDiffBoundsZeroRowFiles(t *testing.T) {
	patch := []byte("diff --git a/a.bin b/a.bin\nBinary files a/a.bin and b/a.bin differ\ndiff --git a/b.bin b/b.bin\nBinary files a/b.bin and b/b.bin differ\n")
	files, truncated, err := parseUnifiedDiff(patch, 1, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || !truncated {
		t.Fatalf("parse = %d files, truncated=%v; want one file and an honest cut", len(files), truncated)
	}
}

func TestParseStatusBoundsTheCompleteCatalog(t *testing.T) {
	if _, _, err := parseStatusZ([]byte("?? a.txt\x00?? b.txt\x00"), 1); !errors.Is(err, ErrResultTooLarge) {
		t.Fatalf("parseStatusZ error = %v, want ErrResultTooLarge", err)
	}
}

func TestApplyNumstatRejectsMalformedCounts(t *testing.T) {
	changes := map[string]*FileChange{"a.txt": {Path: "a.txt"}}
	if err := applyNumstatZ([]byte("invalid\t1\ta.txt\x00"), changes); err == nil {
		t.Fatal("applyNumstatZ accepted a malformed added-line count")
	}
}

func gitTestCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	_ = gitTestCommandOutput(t, dir, args...)
}

func gitTestCommandOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	} else {
		return string(output)
	}
	return ""
}
