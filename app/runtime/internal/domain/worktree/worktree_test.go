package worktree_test

import (
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
