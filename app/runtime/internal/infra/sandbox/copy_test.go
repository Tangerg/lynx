package sandbox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCopyTreePreservesDirectoryAndSymlinkSemantics(t *testing.T) {
	source := t.TempDir()
	destination := t.TempDir()
	if err := os.Mkdir(filepath.Join(source, "nested"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "nested", "note.txt"), []byte("note"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("nested/note.txt", filepath.Join(source, "note-link")); err != nil {
		t.Fatal(err)
	}

	if err := copyTree(t.Context(), source, destination); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(destination, "note-link"))
	if err != nil || string(content) != "note" {
		t.Fatalf("copied symlink content = (%q, %v)", content, err)
	}
	linkInfo, err := os.Lstat(filepath.Join(destination, "note-link"))
	if err != nil || linkInfo.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("copied link info = (%v, %v), want symlink", linkInfo, err)
	}
	directoryInfo, err := os.Stat(filepath.Join(destination, "nested"))
	if err != nil || directoryInfo.Mode().Perm() != 0o750 {
		t.Fatalf("copied directory mode = (%v, %v), want 0750", directoryInfo, err)
	}
}

func TestCopyTreeRejectsEscapingSymlink(t *testing.T) {
	source := t.TempDir()
	if err := os.Symlink("../outside", filepath.Join(source, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := copyTree(t.Context(), source, t.TempDir()); err == nil {
		t.Fatal("escaping source symlink was accepted")
	}
}

func TestCopyTreeRequiresIndependentAbsoluteRoots(t *testing.T) {
	source := t.TempDir()
	insideDestination := filepath.Join(source, "nested")
	if err := os.Mkdir(insideDestination, 0o700); err != nil {
		t.Fatal(err)
	}
	linkedParent := t.TempDir()
	linkedDestination := filepath.Join(linkedParent, "destination")
	if err := os.Symlink(source, linkedDestination); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name        string
		source      string
		destination string
	}{
		{name: "relative source", source: "relative", destination: t.TempDir()},
		{name: "relative destination", source: source, destination: "relative"},
		{name: "destination inside source", source: source, destination: insideDestination},
		{name: "destination resolves to source", source: source, destination: linkedDestination},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := copyTree(t.Context(), test.source, test.destination); err == nil {
				t.Fatal("unsafe workspace roots were accepted")
			}
		})
	}
}

func TestCopyTreeHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := copyTree(ctx, t.TempDir(), t.TempDir())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("copyTree error = %v, want context.Canceled", err)
	}
}
