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

func (database *Database) GetProvider(ctx context.Context, id string) (provider.Provider, bool, error) {
	var baseURL string
	var secret []byte
	err := database.database.QueryRowContext(ctx, `
		SELECT json_extract(body, '$.baseUrl'), secret FROM providers WHERE id = ?`, id,
	).Scan(&baseURL, &secret)
	if errors.Is(err, sql.ErrNoRows) {
		return provider.Provider{}, false, nil
	}
	if err != nil {
		return provider.Provider{}, false, fmt.Errorf("sqlite: get provider %s: %w", id, err)
	}
	value := provider.Provider{ID: id, BaseURL: baseURL, APIKey: string(secret)}
	if len(secret) > 0 {
		value.KeySource = provider.KeyStored
	}
	return value, true, nil
}

func (database *Database) PutProvider(ctx context.Context, value provider.Provider) error {
	body := `{"baseUrl":` + quoteJSON(value.BaseURL) + `}`
	_, err := database.database.ExecContext(ctx, `
		INSERT INTO providers (id, body, secret, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET body = excluded.body, secret = excluded.secret,
			updated_at = excluded.updated_at`,
		value.ID, body, []byte(value.APIKey), encodeTime(time.Now()),
	)
	if err != nil {
		return fmt.Errorf("sqlite: put provider %s: %w", value.ID, err)
	}
	return nil
}

func quoteJSON(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
