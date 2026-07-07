package sqlite

import (
	"context"
	"database/sql"
)

// UtilityRoleStore persists the global utility-model role — the (provider,
// model) the in-house maintenance services (compaction / extraction / titling)
// run on — as a single row. The DB must have been opened via [Open] so the
// utility_role table exists.
type UtilityRoleStore struct {
	store *roleStore
}

// NewUtilityRoleStore wires the given *sql.DB to the utility-role surface.
func NewUtilityRoleStore(db *sql.DB) *UtilityRoleStore {
	return &UtilityRoleStore{store: newRoleStore(db, "utility_role", "utility role", "utility role")}
}

// LoadUtilityRole returns the stored (provider, model); both empty when unset
// (no row yet) — the caller then runs maintenance on the main turn model.
func (s *UtilityRoleStore) LoadUtilityRole(ctx context.Context) (provider, model string, err error) {
	return s.store.load(ctx)
}

// SaveUtilityRole upserts the single utility-role row. An empty model clears
// the role back to the main turn model.
func (s *UtilityRoleStore) SaveUtilityRole(ctx context.Context, provider, model string) error {
	return s.store.save(ctx, provider, model)
}
