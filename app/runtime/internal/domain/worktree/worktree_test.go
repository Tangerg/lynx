package worktree_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/worktree"
)

func TestCanonicalCwdCollapsesSpellings(t *testing.T) {
	dir := t.TempDir()

	want := worktree.CanonicalCwd(dir)
	if want == "" {
		t.Fatal("canonical cwd of a real dir is empty")
	}
	if !filepath.IsAbs(want) {
		t.Errorf("canonical cwd %q is not absolute", want)
	}

	for _, spelling := range []string{
		dir + string(filepath.Separator),
		filepath.Join(dir, "."),
		filepath.Join(dir, "sub", ".."),
	} {
		if got := worktree.CanonicalCwd(spelling); got != want {
			t.Errorf("CanonicalCwd(%q) = %q, want %q", spelling, got, want)
		}
	}

	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if got := worktree.CanonicalCwd(link); got != want {
		t.Errorf("CanonicalCwd(symlink) = %q, want %q", got, want)
	}
}

func TestCanonicalCwdEmpty(t *testing.T) {
	if got := worktree.CanonicalCwd(""); got != "" {
		t.Errorf("CanonicalCwd(\"\") = %q, want empty", got)
	}
}

func TestResolveExistingDir(t *testing.T) {
	dir := t.TempDir()
	got, err := worktree.ResolveExistingDir(filepath.Join(dir, "."))
	if err != nil {
		t.Fatalf("ResolveExistingDir: %v", err)
	}
	if got != worktree.CanonicalCwd(dir) {
		t.Fatalf("ResolveExistingDir = %q, want canonical %q", got, worktree.CanonicalCwd(dir))
	}

	file := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.ResolveExistingDir(file); !errors.Is(err, worktree.ErrNotDirectory) {
		t.Fatalf("ResolveExistingDir(file) err = %v, want ErrNotDirectory", err)
	}

	if _, err := worktree.ResolveExistingDir(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("ResolveExistingDir(missing) err = nil, want error")
	}
}
