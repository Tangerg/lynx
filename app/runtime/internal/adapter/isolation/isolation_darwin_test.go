//go:build darwin

package isolation

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestIsolatorCopiesReusesAndDiscards(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "file.txt"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	iso, err := New(t.TempDir(), t.TempDir(), nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = iso.Close() })
	ctx := context.Background()

	copyDir, err := iso.Workspace(ctx, "s1", project)
	if err != nil {
		t.Fatalf("Workspace: %v", err)
	}
	if copyDir == project {
		t.Fatal("isolated workspace must be a copy, not the project itself")
	}
	// The project's files are present in the copy.
	if got, readFileErr := os.ReadFile(filepath.Join(copyDir, "file.txt")); readFileErr != nil || string(got) != "hello" {
		t.Fatalf("copied file = (%q, %v), want hello", got, readFileErr)
	}
	// A write in the copy does not touch the real project.
	if writeFileErr := os.WriteFile(filepath.Join(copyDir, "scratch.txt"), []byte("x"), 0o600); writeFileErr != nil {
		t.Fatal(writeFileErr)
	}
	if _, statErr := os.Stat(filepath.Join(project, "scratch.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("isolated write leaked into the project: %v", statErr)
	}

	// Same session reuses the same copy (work accumulates across turns).
	again, err := iso.Workspace(ctx, "s1", project)
	if err != nil || again != copyDir {
		t.Fatalf("reuse = (%q, %v), want %q", again, err, copyDir)
	}

	// Discard destroys the copy and is idempotent.
	if err := iso.Discard("s1"); err != nil {
		t.Fatalf("Discard: %v", err)
	}
	if _, err := os.Stat(copyDir); !os.IsNotExist(err) {
		t.Fatalf("copy survived discard: %v", err)
	}
	if err := iso.Discard("s1"); err != nil {
		t.Fatalf("Discard is not idempotent: %v", err)
	}
}
