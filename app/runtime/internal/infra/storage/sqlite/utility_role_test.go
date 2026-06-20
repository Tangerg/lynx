package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

func newUtilityRoleStore(t *testing.T) *sqlite.UtilityRoleStore {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewUtilityRoleStore(db)
}

func TestUtilityRoleStore_RoundTrip(t *testing.T) {
	s := newUtilityRoleStore(t)
	ctx := context.Background()

	// Unset → both empty, no error.
	if p, m, err := s.LoadUtilityRole(ctx); err != nil || p != "" || m != "" {
		t.Fatalf("empty load = (%q, %q, %v); want ('', '', nil)", p, m, err)
	}

	// Save then load round-trips.
	if err := s.SaveUtilityRole(ctx, "anthropic", "claude-haiku-4-5"); err != nil {
		t.Fatalf("save: %v", err)
	}
	if p, m, err := s.LoadUtilityRole(ctx); err != nil || p != "anthropic" || m != "claude-haiku-4-5" {
		t.Fatalf("load = (%q, %q, %v); want (anthropic, claude-haiku-4-5, nil)", p, m, err)
	}

	// Save again upserts the single row (no duplicate, latest wins).
	if err := s.SaveUtilityRole(ctx, "openai", "gpt-5-mini"); err != nil {
		t.Fatalf("re-save: %v", err)
	}
	if p, m, _ := s.LoadUtilityRole(ctx); p != "openai" || m != "gpt-5-mini" {
		t.Fatalf("load after re-save = (%q, %q); want (openai, gpt-5-mini)", p, m)
	}

	// Clearing (empty model) round-trips as unset.
	if err := s.SaveUtilityRole(ctx, "", ""); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if p, m, _ := s.LoadUtilityRole(ctx); p != "" || m != "" {
		t.Fatalf("load after clear = (%q, %q); want empty", p, m)
	}
}
