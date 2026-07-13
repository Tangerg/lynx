package workspacepath_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/workspacepath"
)

func TestCanonicalNormalizesSpellingsAndSymlinks(t *testing.T) {
	dir := t.TempDir()
	want := workspacepath.Canonical(dir)
	for _, spelling := range []string{
		dir + string(filepath.Separator),
		filepath.Join(dir, "."),
		filepath.Join(dir, "sub", ".."),
	} {
		if got := workspacepath.Canonical(spelling); got != want {
			t.Errorf("Canonical(%q) = %q, want %q", spelling, got, want)
		}
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if got := workspacepath.Canonical(link); got != want {
		t.Fatalf("Canonical(symlink) = %q, want %q", got, want)
	}
	if got := workspacepath.Canonical(""); got != "" {
		t.Fatalf("Canonical(empty) = %q", got)
	}
}

func TestResolveExistingDir(t *testing.T) {
	dir := t.TempDir()
	got, err := workspacepath.ResolveExistingDir(filepath.Join(dir, "."))
	if err != nil || got != workspacepath.Canonical(dir) {
		t.Fatalf("ResolveExistingDir = %q, %v", got, err)
	}
	file := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := workspacepath.ResolveExistingDir(file); !errors.Is(err, workspacepath.ErrNotDirectory) {
		t.Fatalf("file error = %v, want ErrNotDirectory", err)
	}
	if _, err := workspacepath.ResolveExistingDir(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("missing directory must fail")
	}
}
