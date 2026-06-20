package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// UtilityRoleStore persists the global utility-model role — the (provider,
// model) the in-house maintenance services (compaction / extraction / titling)
// run on — as a single row. The DB must have been opened via [Open] so the
// utility_role table exists.
type UtilityRoleStore struct {
	db *sql.DB
}

// NewUtilityRoleStore wires the given *sql.DB to the utility-role surface.
func NewUtilityRoleStore(db *sql.DB) *UtilityRoleStore {
	return &UtilityRoleStore{db: db}
}

// LoadUtilityRole returns the stored (provider, model); both empty when unset
// (no row yet) — the caller then runs maintenance on the main turn model.
func (s *UtilityRoleStore) LoadUtilityRole(ctx context.Context) (provider, model string, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT provider, model FROM utility_role WHERE id = 1`).
		Scan(&provider, &model)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("sqlite: load utility role: %w", err)
	}
	return provider, model, nil
}

// SaveUtilityRole upserts the single utility-role row. An empty model clears
// the role back to the main turn model.
func (s *UtilityRoleStore) SaveUtilityRole(ctx context.Context, provider, model string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO utility_role (id, provider, model) VALUES (1, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET provider = excluded.provider, model = excluded.model`,
		provider, model)
	if err != nil {
		return fmt.Errorf("sqlite: save utility role: %w", err)
	}
	return nil
}
