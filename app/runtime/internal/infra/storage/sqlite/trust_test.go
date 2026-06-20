package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

func TestTrustStore_RoundTrip(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	s := sqlite.NewTrustStore(db)

	const root = "/home/me/project"

	// Default: untrusted.
	if ok, _ := s.IsTrusted(ctx, root); ok {
		t.Fatal("a never-trusted project should not be trusted")
	}

	// Trust → trusted; idempotent.
	if err := s.Trust(ctx, root); err != nil {
		t.Fatal(err)
	}
	if err := s.Trust(ctx, root); err != nil {
		t.Fatalf("re-trust should be idempotent: %v", err)
	}
	if ok, _ := s.IsTrusted(ctx, root); !ok {
		t.Fatal("project should be trusted after Trust")
	}
	if list, _ := s.List(ctx); len(list) != 1 || list[0] != root {
		t.Fatalf("List = %v, want [%s]", list, root)
	}

	// Untrust → gone.
	if err := s.Untrust(ctx, root); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.IsTrusted(ctx, root); ok {
		t.Fatal("project should be untrusted after Untrust")
	}
}
