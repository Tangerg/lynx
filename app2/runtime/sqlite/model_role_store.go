package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

func (database *Database) GetModelRole(ctx context.Context, role string, target any) (bool, error) {
	var body string
	err := database.database.QueryRowContext(ctx, `SELECT body FROM model_roles WHERE role = ?`, role).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("sqlite: get %s model role: %w", role, err)
	}
	if err := json.Unmarshal([]byte(body), target); err != nil {
		return false, fmt.Errorf("sqlite: decode %s model role: %w", role, err)
	}
	return true, nil
}

func (database *Database) PutModelRole(ctx context.Context, role string, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("sqlite: encode %s model role: %w", role, err)
	}
	_, err = database.database.ExecContext(ctx, `
		INSERT INTO model_roles (role, body, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(role) DO UPDATE SET body = excluded.body, updated_at = excluded.updated_at`,
		role, string(body), encodeTime(time.Now()),
	)
	if err != nil {
		return fmt.Errorf("sqlite: put %s model role: %w", role, err)
	}
	return nil
}
