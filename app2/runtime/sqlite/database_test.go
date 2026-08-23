package sqlite_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	lyrasqlite "github.com/Tangerg/lynx/app2/runtime/sqlite"
)

func TestDatabaseCreatesOneStableEpochOneIdentity(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "nested", "runtime.db")
	database, err := lyrasqlite.Open(t.Context(), lyrasqlite.Config{Path: path, CreatedByVersion: "dev"})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	first := database.Metadata()
	if first.SchemaEpoch != lyrasqlite.SchemaEpoch || first.StoreID == "" || first.IdempotencyNamespace == "" {
		t.Fatalf("metadata = %+v", first)
	}
	if err := database.Ready(t.Context()); err != nil {
		t.Fatalf("Ready() error = %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}

	reopened, err := lyrasqlite.Open(t.Context(), lyrasqlite.Config{Path: path, CreatedByVersion: "new-build"})
	if err != nil {
		t.Fatalf("reopen error = %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if got := reopened.Metadata(); got != first {
		t.Fatalf("reopened metadata = %+v, want %+v", got, first)
	}
}

func TestDatabaseRejectsCorruptFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "runtime.db")
	if err := os.WriteFile(path, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	_, err := lyrasqlite.Open(t.Context(), lyrasqlite.Config{Path: path, CreatedByVersion: "dev"})
	if err == nil {
		t.Fatal("Open() accepted a corrupt database")
	}
}

func TestDatabaseRequiresAbsolutePath(t *testing.T) {
	t.Parallel()

	_, err := lyrasqlite.Open(t.Context(), lyrasqlite.Config{Path: "runtime.db", CreatedByVersion: "dev"})
	if err == nil || !errors.Is(err, lyrasqlite.ErrInvalidConfig) {
		t.Fatalf("Open() error = %v", err)
	}
}
