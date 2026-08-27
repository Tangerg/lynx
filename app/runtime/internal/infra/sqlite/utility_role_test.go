package sqlite_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Tangerg/scope/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/scope/app/runtime/internal/infra/sqlite"
)

func newUtilityRoleStore(t *testing.T) *sqlite.UtilityRoleStore {
	t.Helper()
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewUtilityRoleStore(db)
}

func TestUtilityRoleStore_RoundTrip(t *testing.T) {
	s := newUtilityRoleStore(t)
	ctx := context.Background()

	// Unset → zero role, no error.
	if role, err := s.LoadUtilityRole(ctx); err != nil || role.Configured() {
		t.Fatalf("empty load = (%+v, %v); want (zero, nil)", role, err)
	}

	// Save then load round-trips.
	if err := s.SaveUtilityRole(ctx, mustStoredRole(t, "anthropic", "claude-haiku-4-5")); err != nil {
		t.Fatalf("save: %v", err)
	}
	if role, err := s.LoadUtilityRole(ctx); err != nil || role.Provider() != "anthropic" || role.Model() != "claude-haiku-4-5" {
		t.Fatalf("load = (%+v, %v); want (anthropic, claude-haiku-4-5, nil)", role, err)
	}

	// Save again upserts the single row (no duplicate, latest wins).
	if err := s.SaveUtilityRole(ctx, mustStoredRole(t, "openai", "gpt-5-mini")); err != nil {
		t.Fatalf("re-save: %v", err)
	}
	if role, _ := s.LoadUtilityRole(ctx); role.Provider() != "openai" || role.Model() != "gpt-5-mini" {
		t.Fatalf("load after re-save = %+v; want (openai, gpt-5-mini)", role)
	}

	// Clearing (zero role) round-trips as unset.
	if err := s.SaveUtilityRole(ctx, modelref.Selection{}); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if role, _ := s.LoadUtilityRole(ctx); role.Configured() {
		t.Fatalf("load after clear = %+v; want empty", role)
	}
}

func TestUtilityRoleStoreRejectsPartialPersistedSelection(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, execErr := db.Exec(`INSERT INTO utility_role (id, provider, model) VALUES (1, ?, ?)`, "anthropic", ""); execErr != nil {
		t.Fatalf("seed corrupt role: %v", execErr)
	}
	_, err = sqlite.NewUtilityRoleStore(db).LoadUtilityRole(context.Background())
	if !errors.Is(err, modelref.ErrIncomplete) {
		t.Fatalf("load partial role error = %v, want %v", err, modelref.ErrIncomplete)
	}
}

func mustStoredRole(t testing.TB, provider, model string) modelref.Selection {
	t.Helper()
	role, err := modelref.New(provider, model)
	if err != nil {
		t.Fatal(err)
	}
	return role
}
