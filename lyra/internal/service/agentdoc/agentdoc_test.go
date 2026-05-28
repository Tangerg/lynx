package agentdoc_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/lyra/internal/service/agentdoc"
)

// writeFile is the test helper for laying out an AGENTS.md fixture.
// All discovery happens against the OS filesystem so we use real
// temp dirs rather than fstest.MapFS.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// mkGitDir places an empty `.git` directory so findProjectRoot
// stops climbing here.
func mkGitDir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(path, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
}

// TestDiscoverWalksProjectTree confirms files at every level
// between cwd and the project root are picked up, ordered root-to-
// leaf.
func TestDiscoverWalksProjectTree(t *testing.T) {
	root := t.TempDir()
	mkGitDir(t, root)
	mid := filepath.Join(root, "pkg")
	leaf := filepath.Join(mid, "feature")
	writeFile(t, filepath.Join(root, "AGENTS.md"), "root-content")
	writeFile(t, filepath.Join(mid, "AGENTS.md"), "mid-content")
	writeFile(t, filepath.Join(leaf, "AGENTS.md"), "leaf-content")

	files, err := agentdoc.Discover(context.Background(), leaf, "")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("len(files) = %d, want 3 (got %+v)", len(files), files)
	}
	// Root-to-leaf order
	if !strings.HasSuffix(files[0].Path, "/AGENTS.md") || files[0].Content != "root-content" {
		t.Fatalf("files[0] = %+v", files[0])
	}
	if files[2].Content != "leaf-content" {
		t.Fatalf("files[2] = %+v", files[2])
	}
}

// TestDiscoverFallsBackToCwdWithoutGit confirms: when no .git is
// found while walking up, root = cwd (only cwd's AGENTS.md is
// considered).
func TestDiscoverFallsBackToCwdWithoutGit(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Dir(dir)
	// parent shouldn't be a git root (it's the system temp); set up
	// AGENTS.md there too — the resolver MUST NOT pick it up.
	writeFile(t, filepath.Join(parent, "AGENTS.md"), "parent-leak")
	writeFile(t, filepath.Join(dir, "AGENTS.md"), "cwd-only")

	files, err := agentdoc.Discover(context.Background(), dir, "")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file (cwd only); got %d: %+v", len(files), files)
	}
	if files[0].Content != "cwd-only" {
		t.Fatalf("files[0].Content = %q", files[0].Content)
	}
}

// TestDiscoverDedupesAbsPath confirms: when the project root file
// and an ancestor candidate resolve to the same abs path, we only
// see it once.
func TestDiscoverDedupesAbsPath(t *testing.T) {
	root := t.TempDir()
	mkGitDir(t, root)
	writeFile(t, filepath.Join(root, "AGENTS.md"), "single")

	// cwd == root: discover should hit the same file once even though
	// dirsRootToLeaf yields a single entry.
	files, err := agentdoc.Discover(context.Background(), root, "")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("len(files) = %d, want 1", len(files))
	}
}

// TestDiscoverPicksLyraSubdirBeforePlainFile confirms the order
// inside one directory: {.lyra/AGENTS.md} first, then {AGENTS.md}.
// Both ARE collected (they're different paths, not a first-match
// pair).
func TestDiscoverPicksLyraSubdirBeforePlainFile(t *testing.T) {
	root := t.TempDir()
	mkGitDir(t, root)
	writeFile(t, filepath.Join(root, ".lyra", "AGENTS.md"), "lyra-subdir")
	writeFile(t, filepath.Join(root, "AGENTS.md"), "plain")

	files, err := agentdoc.Discover(context.Background(), root, "")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("len = %d, want 2", len(files))
	}
	if files[0].Content != "lyra-subdir" {
		t.Fatalf("files[0] = %+v (want lyra-subdir first)", files[0])
	}
	if files[1].Content != "plain" {
		t.Fatalf("files[1] = %+v", files[1])
	}
}

// TestDiscoverFirstMatchAgentsMdLowercase confirms: when both
// AGENTS.md and agents.md exist in the same dir, the first-match
// rule picks AGENTS.md only (the canonical name).
func TestDiscoverFirstMatchAgentsMdLowercase(t *testing.T) {
	root := t.TempDir()
	mkGitDir(t, root)
	writeFile(t, filepath.Join(root, "AGENTS.md"), "upper")
	writeFile(t, filepath.Join(root, "agents.md"), "lower")

	files, err := agentdoc.Discover(context.Background(), root, "")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	// On case-insensitive filesystems (macOS default) both names
	// resolve to the same file — we'd get 1. On case-sensitive
	// filesystems both files exist as separate entries but our
	// tryFirst stops after AGENTS.md, so we still get 1.
	if len(files) != 1 {
		t.Fatalf("len = %d, want 1 (got %+v)", len(files), files)
	}
	if !strings.Contains(files[0].Content, "upper") && !strings.Contains(files[0].Content, "lower") {
		// On case-insensitive fs the content might be either depending
		// on which write landed last.
		t.Fatalf("files[0].Content = %q", files[0].Content)
	}
}

// TestDiscoverUserScope confirms ~/.lyra/AGENTS.md is picked up
// before project files in the render order.
func TestDiscoverUserScope(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	mkGitDir(t, root)
	writeFile(t, filepath.Join(home, ".lyra", "AGENTS.md"), "user-prefs")
	writeFile(t, filepath.Join(root, "AGENTS.md"), "project")

	files, err := agentdoc.Discover(context.Background(), root, home)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("len = %d, want 2 (got %+v)", len(files), files)
	}
	if files[0].Content != "user-prefs" {
		t.Fatalf("files[0] = %+v (want user-prefs first)", files[0])
	}
	if files[1].Content != "project" {
		t.Fatalf("files[1] = %+v", files[1])
	}
}

// TestDiscoverIgnoresEmpty confirms empty / whitespace-only files
// don't appear in the result (they'd just dilute the budget).
func TestDiscoverIgnoresEmpty(t *testing.T) {
	root := t.TempDir()
	mkGitDir(t, root)
	writeFile(t, filepath.Join(root, "AGENTS.md"), "   \n\t  \n")
	writeFile(t, filepath.Join(root, "pkg", "AGENTS.md"), "real-content")

	files, err := agentdoc.Discover(context.Background(), filepath.Join(root, "pkg"), "")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("len = %d, want 1 (got %+v)", len(files), files)
	}
	if files[0].Content != "real-content" {
		t.Fatalf("files[0].Content = %q", files[0].Content)
	}
}

// TestRenderAnnotates confirms each file gets the `<!-- From: -->`
// provenance comment.
func TestRenderAnnotates(t *testing.T) {
	out := agentdoc.Render([]agentdoc.File{
		{Path: "/a/AGENTS.md", Content: "alpha"},
		{Path: "/b/AGENTS.md", Content: "beta"},
	}, agentdoc.DefaultMaxBytes)

	if !strings.Contains(out, "<!-- From: /a/AGENTS.md -->") {
		t.Fatalf("missing /a annotation: %q", out)
	}
	if !strings.Contains(out, "<!-- From: /b/AGENTS.md -->") {
		t.Fatalf("missing /b annotation: %q", out)
	}
	if !strings.Contains(out, "alpha") || !strings.Contains(out, "beta") {
		t.Fatalf("missing content: %q", out)
	}
	if strings.Index(out, "alpha") > strings.Index(out, "beta") {
		t.Fatalf("alpha must precede beta in output")
	}
}

// TestRenderTrimsFromRoot confirms the budget trim discards earliest
// (root-most) files first since deeper = more specific = more
// valuable.
func TestRenderTrimsFromRoot(t *testing.T) {
	files := []agentdoc.File{
		{Path: "/root/AGENTS.md", Content: strings.Repeat("a", 1000)},
		{Path: "/leaf/AGENTS.md", Content: "leaf"},
	}
	out := agentdoc.Render(files, 200) // budget can fit only the second
	if strings.Contains(out, "/root/AGENTS.md") {
		t.Fatalf("root file should have been trimmed: %q", out)
	}
	if !strings.Contains(out, "leaf") {
		t.Fatalf("leaf file should survive: %q", out)
	}
}

// TestRenderReturnsEmptyOnNoInput confirms: empty slice / zero
// budget yield empty string (not a panic, not a stub header).
func TestRenderReturnsEmptyOnNoInput(t *testing.T) {
	if got := agentdoc.Render(nil, agentdoc.DefaultMaxBytes); got != "" {
		t.Fatalf("Render(nil) = %q", got)
	}
	if got := agentdoc.Render([]agentdoc.File{{Path: "/x", Content: "y"}}, 0); got != "" {
		t.Fatalf("Render(..., 0) = %q", got)
	}
}
