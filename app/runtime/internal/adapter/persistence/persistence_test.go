package persistence

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/scope/app/runtime/internal/domain/knowledge"
	sqlitestore "github.com/Tangerg/scope/app/runtime/internal/infra/sqlite"
)

func TestOpenRequiresAndUsesExplicitProcessPaths(t *testing.T) {
	if _, err := Open(t.Context(), Config{DefaultWorkspacePath: t.TempDir()}); err == nil {
		t.Fatal("Open accepted an empty data directory")
	}
	if _, err := Open(t.Context(), Config{DataDirectory: t.TempDir()}); err == nil {
		t.Fatal("Open accepted an empty default workspace path")
	}
	if _, err := Open(t.Context(), Config{DataDirectory: "relative-data", DefaultWorkspacePath: t.TempDir()}); err == nil {
		t.Fatal("Open accepted a relative data directory")
	}
	if _, err := Open(t.Context(), Config{DataDirectory: t.TempDir(), DefaultWorkspacePath: "relative-workspace"}); err == nil {
		t.Fatal("Open accepted a relative default workspace path")
	}

	dataDirectory := filepath.Join(t.TempDir(), "data")
	defaultWorkspace := t.TempDir()
	bundle, err := Open(t.Context(), Config{
		DataDirectory: dataDirectory, DefaultWorkspacePath: defaultWorkspace,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = bundle.Close() })
	if bundle.DataDirectory != dataDirectory {
		t.Fatalf("DataDirectory = %q, want %q", bundle.DataDirectory, dataDirectory)
	}
	if bundle.IdempotencyNamespace == "" {
		t.Fatal("Open returned an empty idempotency namespace")
	}
	if _, statErr := os.Stat(filepath.Join(dataDirectory, "scopeapp.db")); statErr != nil {
		t.Fatalf("data directory does not own scopeapp.db: %v", statErr)
	}
	fresh, err := bundle.Knowledge.Get(t.Context(), knowledge.ScopeCWD, "")
	if err != nil {
		t.Fatalf("read default project knowledge: %v", err)
	}
	if _, err := bundle.Knowledge.Update(
		t.Context(), knowledge.ScopeCWD, "", fresh.Revision, "project",
	); err != nil {
		t.Fatalf("write default project knowledge: %v", err)
	}
	if _, err := os.Stat(filepath.Join(defaultWorkspace, "SCOPEAPP.md")); err != nil {
		t.Fatalf("default workspace does not own project knowledge: %v", err)
	}
}

func TestBundleCloseIsIdempotent(t *testing.T) {
	db, err := sqlitestore.Open(t.Context(), filepath.Join(t.TempDir(), "scopeapp.db"))
	if err != nil {
		t.Fatal(err)
	}
	bundle := &Bundle{db: db}
	if err := bundle.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := bundle.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if err := db.Ping(); err == nil {
		t.Fatal("database remained usable after Bundle.Close")
	}
}
