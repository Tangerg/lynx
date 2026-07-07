package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

func newEmbeddingRoleStore(t *testing.T) *sqlite.EmbeddingRoleStore {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewEmbeddingRoleStore(db)
}

func TestEmbeddingRoleStore_RoundTrip(t *testing.T) {
	s := newEmbeddingRoleStore(t)
	ctx := context.Background()

	if p, m, err := s.LoadEmbeddingRole(ctx); err != nil || p != "" || m != "" {
		t.Fatalf("empty load = (%q, %q, %v); want ('', '', nil)", p, m, err)
	}

	if err := s.SaveEmbeddingRole(ctx, "openai", "text-embedding-3-small"); err != nil {
		t.Fatalf("save: %v", err)
	}
	if p, m, err := s.LoadEmbeddingRole(ctx); err != nil || p != "openai" || m != "text-embedding-3-small" {
		t.Fatalf("load = (%q, %q, %v); want (openai, text-embedding-3-small, nil)", p, m, err)
	}

	if err := s.SaveEmbeddingRole(ctx, "anthropic", "voyage-3-large"); err != nil {
		t.Fatalf("re-save: %v", err)
	}
	if p, m, err := s.LoadEmbeddingRole(ctx); err != nil || p != "anthropic" || m != "voyage-3-large" {
		t.Fatalf("load after re-save = (%q, %q, %v); want (anthropic, voyage-3-large, nil)", p, m, err)
	}

	if err := s.SaveEmbeddingRole(ctx, "", ""); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if p, m, err := s.LoadEmbeddingRole(ctx); err != nil || p != "" || m != "" {
		t.Fatalf("load after clear = (%q, %q, %v); want empty", p, m, err)
	}
}
