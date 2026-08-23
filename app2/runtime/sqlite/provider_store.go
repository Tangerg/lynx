package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/domain/provider"
)

func (database *Database) GetProvider(ctx context.Context, id string) (provider.Configuration, bool, error) {
	var baseURL string
	var secret []byte
	var revision uint64
	err := database.database.QueryRowContext(ctx, `
		SELECT json_extract(body, '$.baseUrl'), secret, revision FROM providers WHERE id = ?`, id,
	).Scan(&baseURL, &secret, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return provider.Configuration{}, false, nil
	}
	if err != nil {
		return provider.Configuration{}, false, fmt.Errorf("sqlite: get provider %s: %w", id, err)
	}
	value, err := provider.Rehydrate(id, baseURL, string(secret), revision)
	if err != nil {
		return provider.Configuration{}, false, fmt.Errorf("sqlite: restore provider %s: %w", id, err)
	}
	return value, true, nil
}

func (database *Database) SaveProvider(ctx context.Context, value provider.Configuration, previousRevision uint64) error {
	body := `{"baseUrl":` + quoteJSON(value.BaseURL()) + `}`
	var result sql.Result
	var err error
	if previousRevision == 0 {
		result, err = database.database.ExecContext(ctx, `
			INSERT INTO providers (id, body, secret, revision, updated_at)
			VALUES (?, ?, ?, ?, ?) ON CONFLICT(id) DO NOTHING`,
			value.ID(), body, []byte(value.APIKey()), value.Revision(), encodeTime(time.Now()),
		)
	} else {
		result, err = database.database.ExecContext(ctx, `
			UPDATE providers SET body = ?, secret = ?, revision = ?, updated_at = ?
			WHERE id = ? AND revision = ?`,
			body, []byte(value.APIKey()), value.Revision(), encodeTime(time.Now()),
			value.ID(), previousRevision,
		)
	}
	if err != nil {
		return fmt.Errorf("sqlite: save provider %s: %w", value.ID(), err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect provider save %s: %w", value.ID(), err)
	}
	if changed == 0 {
		return provider.ErrRevisionConflict
	}
	return nil
}

func quoteJSON(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
