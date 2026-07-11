package promptsource_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/promptsource"
)

// writeFile is the test helper for laying out an AGENTS.md fixture. All discovery
// happens against the OS filesystem so we use real temp dirs rather than
// fstest.MapFS.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// mkGitDir places an empty `.git` directory so the project-root walk stops here.
func mkGitDir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(path, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
}

func TestDiscoverAgentDocsWalksProjectTree(t *testing.T) {
	root := t.TempDir()
	mkGitDir(t, root)
	mid := filepath.Join(root, "pkg")
	leaf := filepath.Join(mid, "feature")
	writeFile(t, filepath.Join(root, "AGENTS.md"), "root-content")
	writeFile(t, filepath.Join(mid, "AGENTS.md"), "mid-content")
	writeFile(t, filepath.Join(leaf, "AGENTS.md"), "leaf-content")

	files, err := promptsource.DiscoverAgentDocs(context.Background(), leaf, "")
	if err != nil {
		t.Fatalf("DiscoverAgentDocs: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("len(files) = %d, want 3 (got %+v)", len(files), files)
	}
	if !strings.HasSuffix(files[0].Path, "/AGENTS.md") || files[0].Content != "root-content" {
		t.Fatalf("files[0] = %+v", files[0])
	}
	if files[2].Content != "leaf-content" {
		t.Fatalf("files[2] = %+v", files[2])
	}
}

func TestDiscoverAgentDocsFallsBackToCwdWithoutGit(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Dir(dir)
	writeFile(t, filepath.Join(parent, "AGENTS.md"), "parent-leak")
	writeFile(t, filepath.Join(dir, "AGENTS.md"), "cwd-only")

	files, err := promptsource.DiscoverAgentDocs(context.Background(), dir, "")
	if err != nil {
		t.Fatalf("DiscoverAgentDocs: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file (cwd only); got %d: %+v", len(files), files)
	}
	if files[0].Content != "cwd-only" {
		t.Fatalf("files[0].Content = %q", files[0].Content)
	}
}

func TestDiscoverAgentDocsDedupesAbsPath(t *testing.T) {
	root := t.TempDir()
	mkGitDir(t, root)
	writeFile(t, filepath.Join(root, "AGENTS.md"), "single")

	files, err := promptsource.DiscoverAgentDocs(context.Background(), root, "")
	if err != nil {
		t.Fatalf("DiscoverAgentDocs: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("len(files) = %d, want 1", len(files))
	}
}

func TestDiscoverAgentDocsPicksLyraSubdirBeforePlainFile(t *testing.T) {
	root := t.TempDir()
	mkGitDir(t, root)
	writeFile(t, filepath.Join(root, ".lyra", "AGENTS.md"), "lyra-subdir")
	writeFile(t, filepath.Join(root, "AGENTS.md"), "plain")

	files, err := promptsource.DiscoverAgentDocs(context.Background(), root, "")
	if err != nil {
		t.Fatalf("DiscoverAgentDocs: %v", err)
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

func TestDiscoverAgentDocsFirstMatchAgentsMdLowercase(t *testing.T) {
	root := t.TempDir()
	mkGitDir(t, root)
	writeFile(t, filepath.Join(root, "AGENTS.md"), "upper")
	writeFile(t, filepath.Join(root, "agents.md"), "lower")

	files, err := promptsource.DiscoverAgentDocs(context.Background(), root, "")
	if err != nil {
		t.Fatalf("DiscoverAgentDocs: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("len = %d, want 1 (got %+v)", len(files), files)
	}
	if !strings.Contains(files[0].Content, "upper") && !strings.Contains(files[0].Content, "lower") {
		t.Fatalf("files[0].Content = %q", files[0].Content)
	}
}

func TestDiscoverAgentDocsUserScope(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	mkGitDir(t, root)
	writeFile(t, filepath.Join(home, ".lyra", "AGENTS.md"), "user-prefs")
	writeFile(t, filepath.Join(root, "AGENTS.md"), "project")

	files, err := promptsource.DiscoverAgentDocs(context.Background(), root, home)
	if err != nil {
		t.Fatalf("DiscoverAgentDocs: %v", err)
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

func TestDiscoverAgentDocsIgnoresEmpty(t *testing.T) {
	root := t.TempDir()
	mkGitDir(t, root)
	writeFile(t, filepath.Join(root, "AGENTS.md"), "   \n\t  \n")
	writeFile(t, filepath.Join(root, "pkg", "AGENTS.md"), "real-content")

	files, err := promptsource.DiscoverAgentDocs(context.Background(), filepath.Join(root, "pkg"), "")
	if err != nil {
		t.Fatalf("DiscoverAgentDocs: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("len = %d, want 1 (got %+v)", len(files), files)
	}
	if files[0].Content != "real-content" {
		t.Fatalf("files[0].Content = %q", files[0].Content)
	}
}
