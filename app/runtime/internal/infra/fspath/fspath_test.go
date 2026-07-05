package fspath_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/infra/fspath"
)

// TestCanonicalCollapsesSpellings: different spellings of one directory
// (trailing slash, an inner "." segment, a symlinked ancestor) must collapse to
// a single string, so callers keying a map / lock by cwd can't take two keys for
// one location.
func TestCanonicalCollapsesSpellings(t *testing.T) {
	dir := t.TempDir()

	want := fspath.Canonical(dir)
	if want == "" {
		t.Fatal("canonical of a real dir is empty")
	}
	if !filepath.IsAbs(want) {
		t.Errorf("canonical %q is not absolute", want)
	}

	for _, spelling := range []string{
		dir + string(filepath.Separator), // trailing slash
		filepath.Join(dir, "."),          // inner "." segment
		filepath.Join(dir, "sub", ".."),  // round-trip through a child
	} {
		if got := fspath.Canonical(spelling); got != want {
			t.Errorf("Canonical(%q) = %q, want %q", spelling, got, want)
		}
	}

	// A symlink to the dir resolves to the same canonical target.
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if got := fspath.Canonical(link); got != want {
		t.Errorf("Canonical(symlink) = %q, want %q", got, want)
	}
}

// TestCanonicalEmpty: the canonical form of "no path" is "no path" — callers
// rely on this to leave an absent cwd absent (not the process cwd).
func TestCanonicalEmpty(t *testing.T) {
	if got := fspath.Canonical(""); got != "" {
		t.Errorf("Canonical(\"\") = %q, want empty", got)
	}
}
