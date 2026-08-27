package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Tangerg/scope/app/runtime/internal/domain/modelref"
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

func (r *roleStore) load(ctx context.Context) (modelref.Selection, error) {
	query := fmt.Sprintf("SELECT provider, model FROM %s WHERE id = 1", r.table)
	var provider, model string
	err := conn(ctx, r.db).QueryRowContext(ctx, query).Scan(&provider, &model)
	if errors.Is(err, sql.ErrNoRows) {
		return modelref.Selection{}, nil
	}
	if err != nil {
		return modelref.Selection{}, fmt.Errorf("sqlite: load %s: %w", r.label, err)
	}
	role, err := modelref.New(provider, model)
	if err != nil {
		return modelref.Selection{}, fmt.Errorf("sqlite: decode %s: %w", r.label, err)
	}
	return role, nil
}

func (r *roleStore) save(ctx context.Context, role modelref.Selection) error {
	query := fmt.Sprintf(
		`INSERT INTO %s (id, provider, model) VALUES (1, ?, ?) ON CONFLICT(id) DO UPDATE SET provider = excluded.provider, model = excluded.model`,
		r.table,
	)
	_, err := conn(ctx, r.db).ExecContext(ctx, query, role.Provider(), role.Model())
	if err != nil {
		return fmt.Errorf("sqlite: save %s: %w", r.label, err)
	}
	return nil
}
