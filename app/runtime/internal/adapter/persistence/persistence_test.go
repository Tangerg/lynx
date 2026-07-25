package persistence

import (
	"context"
	"errors"
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

func TestBundleShutdownHonorsExpiredContext(t *testing.T) {
	db, err := sqlitestore.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatal(err)
	}
	bundle := &Bundle{db: db}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := bundle.Shutdown(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Shutdown error = %v, want context.Canceled", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("expired shutdown closed database: %v", err)
	}
	if err := bundle.Close(); err != nil {
		t.Fatalf("cleanup Close: %v", err)
	}
}
