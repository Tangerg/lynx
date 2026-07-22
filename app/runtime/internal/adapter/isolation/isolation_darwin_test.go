//go:build darwin

package isolation

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// memStore is an in-memory sandbox.SnapshotStore for the discard snapshot.
type memStore struct {
	mu   sync.Mutex
	blob map[string][]byte
}

func (m *memStore) SaveSandboxSnapshot(_ context.Context, id string, archive []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.blob == nil {
		m.blob = map[string][]byte{}
	}
	m.blob[id] = archive
	return nil
}

func (m *memStore) LoadSandboxSnapshot(_ context.Context, id string) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.blob[id]
	return b, ok, nil
}

func TestIsolatorCopiesReusesAndDiscards(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "file.txt"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	iso := New(t.TempDir(), &memStore{}, nil)
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
	if got, err := os.ReadFile(filepath.Join(copyDir, "file.txt")); err != nil || string(got) != "hello" {
		t.Fatalf("copied file = (%q, %v), want hello", got, err)
	}
	// A write in the copy does not touch the real project.
	if err := os.WriteFile(filepath.Join(copyDir, "scratch.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(project, "scratch.txt")); !os.IsNotExist(err) {
		t.Fatalf("isolated write leaked into the project: %v", err)
	}

	// Same session reuses the same copy (work accumulates across turns).
	again, err := iso.Workspace(ctx, "s1", project)
	if err != nil || again != copyDir {
		t.Fatalf("reuse = (%q, %v), want %q", again, err, copyDir)
	}

	// Discard destroys the copy and is idempotent.
	if err := iso.Discard(ctx, "s1"); err != nil {
		t.Fatalf("Discard: %v", err)
	}
	if _, err := os.Stat(copyDir); !os.IsNotExist(err) {
		t.Fatalf("copy survived discard: %v", err)
	}
	if err := iso.Discard(ctx, "s1"); err != nil {
		t.Fatalf("Discard is not idempotent: %v", err)
	}
}
