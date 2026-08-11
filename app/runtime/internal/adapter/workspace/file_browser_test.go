package workspace

import (
	"os"
	"path/filepath"
	"testing"

	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
)

func TestFileBrowserGrepReturnsStableRootRelativePaths(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "alpha.txt"), []byte("first\nneedle\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "nested", "bravo.txt"), []byte("needle\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := (FileBrowser{}).Grep(t.Context(), root, workspaceapp.GrepInput{Query: "needle", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 2 || len(got.Matches) != 2 {
		t.Fatalf("Grep = %+v, want two matches", got)
	}
	if got.Matches[0].Path != "alpha.txt" || got.Matches[0].LineNumber != 2 || got.Matches[1].Path != "nested/bravo.txt" || got.Matches[1].LineNumber != 1 {
		t.Fatalf("matches = %+v, want stable root-relative alpha then nested/bravo", got.Matches)
	}
	for _, match := range got.Matches {
		if filepath.IsAbs(match.Path) {
			t.Fatalf("match leaked an absolute host path: %+v", match)
		}
	}
}

func TestRootRelativeGrepPathRejectsEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	path := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(path, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rootRelativeGrepPath(root, canonicalRoot, path); err == nil {
		t.Fatal("rootRelativeGrepPath accepted a path outside the workspace")
	}
}
