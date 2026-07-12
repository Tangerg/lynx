package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// roleStore is the shared persistence primitive for single-row role tables,
// used by utility-role and embedding-role storage.
type roleStore struct {
	db    *sql.DB
	table string
	label string // role name woven into load/save error context
}

func newRoleStore(db *sql.DB, table, label string) *roleStore {
	return &roleStore{db: db, table: table, label: label}
}

func (s *roleStore) load(ctx context.Context) (provider, model string, err error) {
	query := fmt.Sprintf("SELECT provider, model FROM %s WHERE id = 1", s.table)
	err = s.db.QueryRowContext(ctx, query).Scan(&provider, &model)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("sqlite: load %s: %w", s.label, err)
	}
	return provider, model, nil
}

func (s *roleStore) save(ctx context.Context, provider, model string) error {
	query := fmt.Sprintf(
		`INSERT INTO %s (id, provider, model) VALUES (1, ?, ?) ON CONFLICT(id) DO UPDATE SET provider = excluded.provider, model = excluded.model`,
		s.table,
	)
	_, err := s.db.ExecContext(ctx, query, provider, model)
	if err != nil {
		return fmt.Errorf("sqlite: save %s: %w", s.label, err)
	}
	return nil
}
