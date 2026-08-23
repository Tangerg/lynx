package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/domain/modelselection"
)

type storedModelRole struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

func (database *Database) GetModelRole(ctx context.Context, role modelselection.Role) (modelselection.Selection, bool, error) {
	if !role.Valid() {
		return modelselection.Selection{}, false, fmt.Errorf("sqlite: invalid model role %q", role)
	}
	var body string
	err := database.database.QueryRowContext(ctx, `SELECT body FROM model_roles WHERE role = ?`, role).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return modelselection.Selection{}, false, nil
	}
	if err != nil {
		return modelselection.Selection{}, false, fmt.Errorf("sqlite: get %s model role: %w", role, err)
	}
	var stored storedModelRole
	if err := json.Unmarshal([]byte(body), &stored); err != nil {
		return modelselection.Selection{}, false, fmt.Errorf("sqlite: decode %s model role: %w", role, err)
	}
	selection, err := modelselection.New(stored.Provider, stored.Model)
	if err != nil {
		return modelselection.Selection{}, false, fmt.Errorf("sqlite: restore %s model role: %w", role, err)
	}
	return selection, true, nil
}

func (database *Database) PutModelRole(ctx context.Context, role modelselection.Role, value modelselection.Selection) error {
	if !role.Valid() {
		return fmt.Errorf("sqlite: invalid model role %q", role)
	}
	body, err := json.Marshal(storedModelRole{Provider: value.Provider(), Model: value.Model()})
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
