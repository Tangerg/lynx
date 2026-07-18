package sqlite_test

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

func TestSandboxSnapshotStoreIsImmutable(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewSandboxSnapshotStore(db)
	if err := store.SaveSandboxSnapshot(t.Context(), "sha256:one", []byte("archive")); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSandboxSnapshot(t.Context(), "sha256:one", []byte("archive")); err != nil {
		t.Fatalf("idempotent save: %v", err)
	}
	if err := store.SaveSandboxSnapshot(t.Context(), "sha256:one", []byte("different")); err == nil {
		t.Fatal("same id accepted different snapshot content")
	}
	archive, found, err := store.LoadSandboxSnapshot(t.Context(), "sha256:one")
	if err != nil || !found || !bytes.Equal(archive, []byte("archive")) {
		t.Fatalf("Load = (%q, %v, %v)", archive, found, err)
	}
	if _, found, err := store.LoadSandboxSnapshot(t.Context(), "missing"); err != nil || found {
		t.Fatalf("missing Load = (found %v, err %v)", found, err)
	}
}
