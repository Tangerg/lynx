package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelrole"
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

	if role, err := s.LoadEmbeddingRole(ctx); err != nil || role.Configured() {
		t.Fatalf("empty load = (%+v, %v); want (zero, nil)", role, err)
	}

	if err := s.SaveEmbeddingRole(ctx, mustStoredRole(t, "openai", "text-embedding-3-small")); err != nil {
		t.Fatalf("save: %v", err)
	}
	if role, err := s.LoadEmbeddingRole(ctx); err != nil || role.ProviderID() != "openai" || role.Model() != "text-embedding-3-small" {
		t.Fatalf("load = (%+v, %v); want (openai, text-embedding-3-small, nil)", role, err)
	}

	if err := s.SaveEmbeddingRole(ctx, mustStoredRole(t, "anthropic", "voyage-3-large")); err != nil {
		t.Fatalf("re-save: %v", err)
	}
	if role, err := s.LoadEmbeddingRole(ctx); err != nil || role.ProviderID() != "anthropic" || role.Model() != "voyage-3-large" {
		t.Fatalf("load after re-save = (%+v, %v); want (anthropic, voyage-3-large, nil)", role, err)
	}

	if err := s.SaveEmbeddingRole(ctx, modelrole.Role{}); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if role, err := s.LoadEmbeddingRole(ctx); err != nil || role.Configured() {
		t.Fatalf("load after clear = (%+v, %v); want empty", role, err)
	}
}
