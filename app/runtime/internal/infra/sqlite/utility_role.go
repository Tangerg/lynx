package sqlite

import (
	"context"
	"database/sql"

	"github.com/Tangerg/scope/app/runtime/internal/domain/modelref"
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
	return &UtilityRoleStore{store: newRoleStore(db, "utility_role", "utility role")}
}

// LoadUtilityRole returns the stored role, or its zero value when unset (no
// row yet) — the caller then runs maintenance on the main Run model.
func (u *UtilityRoleStore) LoadUtilityRole(ctx context.Context) (modelref.Selection, error) {
	return u.store.load(ctx)
}

// SaveUtilityRole upserts the single utility-role row. A zero role clears it
// back to the main Run model.
func (u *UtilityRoleStore) SaveUtilityRole(ctx context.Context, role modelref.Selection) error {
	return u.store.save(ctx, role)
}
