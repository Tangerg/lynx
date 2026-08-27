package localruntime

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestDefaultDataDirectoryOwnsLocalDeploymentLayout(t *testing.T) {
	home := t.TempDir()
	directory, err := DefaultDataDirectory(home)
	if err != nil {
		t.Fatal(err)
	}
	wantRoot := filepath.Join(home, ".scopeapp")
	if directory.Path() != wantRoot {
		t.Fatalf("path = %q, want %q", directory.Path(), wantRoot)
	}
	if directory.DatabasePath() != filepath.Join(wantRoot, "scopeapp.db") {
		t.Fatalf("database path = %q", directory.DatabasePath())
	}
	if directory.LocalTokenPath() != filepath.Join(wantRoot, "local-token") {
		t.Fatalf("token path = %q", directory.LocalTokenPath())
	}
}

func TestDataDirectoryRejectsUnownedPaths(t *testing.T) {
	for _, path := range []string{"", "relative"} {
		if _, err := DataDirectoryAt(path); !errors.Is(err, ErrInvalidDataDirectory) {
			t.Fatalf("DataDirectoryAt(%q) error = %v", path, err)
		}
	}
	if _, err := DefaultDataDirectory(""); !errors.Is(err, ErrInvalidDataDirectory) {
		t.Fatalf("DefaultDataDirectory error = %v", err)
	}
}
