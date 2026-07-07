package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
)

// ProviderStore implements provider.Registry against a SQLite database.
// One row per provider id; Configure is an upsert. The DB must have been
// opened via [Open] so the providers table exists.
type ProviderStore struct {
	db *sql.DB
}

var _ provider.Registry = (*ProviderStore)(nil)

// NewProviderStore wires the given *sql.DB to the provider.Registry surface.
func NewProviderStore(db *sql.DB) *ProviderStore {
	return &ProviderStore{db: db}
}

func (s *ProviderStore) List(ctx context.Context) ([]provider.Provider, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, api_key, base_url FROM providers ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list providers: %w", err)
	}
	defer rows.Close()

	var out []provider.Provider
	for rows.Next() {
		var p provider.Provider
		if err := rows.Scan(&p.ID, &p.APIKey, &p.BaseURL); err != nil {
			return nil, fmt.Errorf("sqlite: scan provider: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *ProviderStore) Get(ctx context.Context, id string) (provider.Provider, bool, error) {
	var p provider.Provider
	err := s.db.QueryRowContext(ctx,
		`SELECT id, api_key, base_url FROM providers WHERE id = ?`, id).
		Scan(&p.ID, &p.APIKey, &p.BaseURL)
	if errors.Is(err, sql.ErrNoRows) {
		return provider.Provider{}, false, nil
	}
	if err != nil {
		return provider.Provider{}, false, fmt.Errorf("sqlite: get provider: %w", err)
	}
	return p, true, nil
}

func (s *ProviderStore) Configure(ctx context.Context, p provider.Provider) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO providers (id, api_key, base_url) VALUES (?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET api_key = excluded.api_key, base_url = excluded.base_url`,
		p.ID, p.APIKey, p.BaseURL)
	if err != nil {
		return fmt.Errorf("sqlite: configure provider: %w", err)
	}
	return nil
}
