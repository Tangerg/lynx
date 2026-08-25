package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
)

func TestFileBrowserReadOwnsSourceLineAndWindowLimits(t *testing.T) {
	t.Run("source", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "oversized.txt")
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := file.Truncate(workspaceapp.MaxFileReadSourceBytes + 1); err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		_, err = (FileBrowser{}).Read(t.Context(), root, workspaceapp.FileReadInput{Path: "oversized.txt"})
		if !errors.Is(err, workspaceapp.ErrFileReadTooLarge) {
			t.Fatalf("oversized source error = %v, want ErrFileReadTooLarge", err)
		}
	})

	t.Run("line", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(
			filepath.Join(root, "line.txt"), []byte(strings.Repeat("x", workspaceapp.MaxFileReadLineBytes+1)), 0o644,
		); err != nil {
			t.Fatal(err)
		}
		_, err := (FileBrowser{}).Read(t.Context(), root, workspaceapp.FileReadInput{Path: "line.txt"})
		if !errors.Is(err, workspaceapp.ErrFileReadTooLarge) {
			t.Fatalf("oversized line error = %v, want ErrFileReadTooLarge", err)
		}
	})

	t.Run("range", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "short.txt"), []byte("first\nsecond"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := (FileBrowser{}).Read(t.Context(), root, workspaceapp.FileReadInput{
			Path: "short.txt", StartLine: 3, MaxBytes: workspaceapp.DefaultFileReadBytes,
		})
		if !errors.Is(err, workspaceapp.ErrInvalidFileRange) {
			t.Fatalf("outside range error = %v, want ErrInvalidFileRange", err)
		}
	})
}

func TestFileBrowserReadClipsAtUTF8BoundaryAndPreservesTextShape(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "utf8.txt"), []byte("\xef\xbb\xbféé\r\nlast\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := (FileBrowser{}).Read(t.Context(), root, workspaceapp.FileReadInput{Path: "utf8.txt", MaxBytes: 3})
	if err != nil {
		t.Fatal(err)
	}
	if got.Content != "é" || got.StartLine != 0 || got.EndLine != 1 || got.TotalLines != 3 || !got.Truncated {
		t.Fatalf("Read = %+v, want one valid UTF-8 prefix and honest normalized shape", got)
	}
}

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

func TestFileBrowserGrepReturnsExactTotalBeyondSharedExecutorCountCap(t *testing.T) {
	root := t.TempDir()
	for index := range 300 {
		name := fmt.Sprintf("match-%03d.txt", index)
		if err := os.WriteFile(filepath.Join(root, name), []byte("needle\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := (FileBrowser{}).Grep(t.Context(), root, workspaceapp.GrepInput{Query: "needle", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 300 || len(got.Matches) != 1 {
		t.Fatalf("Grep = %d retained / total %d, want 1/300", len(got.Matches), got.Total)
	}
}

func TestFileBrowserGrepBoundsRetainedMatchMaterial(t *testing.T) {
	root := t.TempDir()
	line := "needle" + strings.Repeat("x", (1<<20)-len("needle"))
	for index := range 9 {
		name := fmt.Sprintf("large-%02d.txt", index)
		if err := os.WriteFile(filepath.Join(root, name), []byte(line), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := (FileBrowser{}).Grep(t.Context(), root, workspaceapp.GrepInput{Query: "needle", Limit: 9})
	if err != nil {
		t.Fatal(err)
	}
	material := 0
	for _, match := range got.Matches {
		material += len(match.Path) + len(match.Text)
	}
	if got.Total != 9 || material > 8<<20 || len(got.Matches) >= got.Total {
		t.Fatalf("Grep = %d retained / total %d / %d bytes, want exact total and an 8 MiB whole-row prefix", len(got.Matches), got.Total, material)
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
