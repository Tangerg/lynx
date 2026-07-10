package persistence

import (
	"path/filepath"
	"testing"

	sqlitestore "github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

func TestBundleCloseIsIdempotent(t *testing.T) {
	db, err := sqlitestore.Open(filepath.Join(t.TempDir(), "lyra.db"))
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
