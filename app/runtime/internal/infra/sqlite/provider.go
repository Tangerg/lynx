package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
)

// ProviderStore persists provider configuration in SQLite. One row per
// provider id; Update is an atomic partial upsert. The DB must have been
// opened via [Open] so the providers table exists.
type ProviderStore struct {
	db *sql.DB
}

// NewProviderStore binds provider persistence to db. Application consumer
// interfaces are satisfied structurally without an Infra-to-Application import.
func NewProviderStore(db *sql.DB) *ProviderStore {
	return &ProviderStore{db: db}
}

func (p *ProviderStore) List(ctx context.Context) ([]provider.Provider, error) {
	rows, err := conn(ctx, p.db).QueryContext(ctx,
		`SELECT id, api_key, base_url FROM providers ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list providers: %w", err)
	}
	defer rows.Close()

	var out []provider.Provider
	for rows.Next() {
		var record provider.Provider
		if err := rows.Scan(&record.ID, &record.APIKey, &record.BaseURL); err != nil {
			return nil, fmt.Errorf("sqlite: scan provider: %w", err)
		}
		out = append(out, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list providers: %w", err)
	}
	return out, nil
}

func (p *ProviderStore) Get(ctx context.Context, id string) (provider.Provider, bool, error) {
	var record provider.Provider
	err := conn(ctx, p.db).QueryRowContext(ctx,
		`SELECT id, api_key, base_url FROM providers WHERE id = ?`, id).
		Scan(&record.ID, &record.APIKey, &record.BaseURL)
	if errors.Is(err, sql.ErrNoRows) {
		return provider.Provider{}, false, nil
	}
	if err != nil {
		return provider.Provider{}, false, fmt.Errorf("sqlite: get provider: %w", err)
	}
	return record, true, nil
}

func (p *ProviderStore) Update(ctx context.Context, id string, patch provider.Patch) (provider.Provider, error) {
	var apiKey, baseURL string
	updateAPIKey := patch.APIKey != nil
	if updateAPIKey {
		apiKey = *patch.APIKey
	}
	updateBaseURL := patch.BaseURL != nil
	if updateBaseURL {
		baseURL = *patch.BaseURL
	}

	var out provider.Provider
	err := conn(ctx, p.db).QueryRowContext(ctx,
		`INSERT INTO providers (id, api_key, base_url) VALUES (?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   api_key = CASE WHEN ? THEN excluded.api_key ELSE providers.api_key END,
		   base_url = CASE WHEN ? THEN excluded.base_url ELSE providers.base_url END
		 RETURNING id, api_key, base_url`,
		id, apiKey, baseURL, updateAPIKey, updateBaseURL,
	).Scan(&out.ID, &out.APIKey, &out.BaseURL)
	if err != nil {
		return provider.Provider{}, fmt.Errorf("sqlite: update provider: %w", err)
	}
	return out, nil
}
