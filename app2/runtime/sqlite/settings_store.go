package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/domain/settings"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

func (database *Database) GetApprovalMode(ctx context.Context) (protocol.ApprovalMode, error) {
	var body string
	err := database.database.QueryRowContext(ctx, `SELECT body FROM approval_state WHERE singleton = 1`).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return protocol.ApprovalModeBalanced, nil
	}
	if err != nil {
		return "", fmt.Errorf("sqlite: get approval mode: %w", err)
	}
	var value struct{ Mode protocol.ApprovalMode `json:"mode"` }
	if err := json.Unmarshal([]byte(body), &value); err != nil {
		return "", fmt.Errorf("sqlite: decode approval mode: %w", err)
	}
	return value.Mode, nil
}

func (database *Database) SetApprovalMode(ctx context.Context, mode protocol.ApprovalMode) error {
	body, _ := json.Marshal(struct{ Mode protocol.ApprovalMode `json:"mode"` }{Mode: mode})
	_, err := database.database.ExecContext(ctx, `
		INSERT INTO approval_state (singleton, body, updated_at) VALUES (1, ?, ?)
		ON CONFLICT(singleton) DO UPDATE SET body = excluded.body, updated_at = excluded.updated_at`,
		string(body), encodeTime(time.Now()))
	if err != nil {
		return fmt.Errorf("sqlite: set approval mode: %w", err)
	}
	return nil
}

func (database *Database) ListApprovalRules(ctx context.Context) ([]protocol.ApprovalRule, error) {
	rows, err := database.database.QueryContext(ctx, `SELECT body FROM approval_rules ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list approval rules: %w", err)
	}
	defer rows.Close()
	values := make([]protocol.ApprovalRule, 0)
	for rows.Next() {
		var body string
		if err := rows.Scan(&body); err != nil {
			return nil, fmt.Errorf("sqlite: scan approval rule: %w", err)
		}
		var value protocol.ApprovalRule
		if err := json.Unmarshal([]byte(body), &value); err != nil {
			return nil, fmt.Errorf("sqlite: decode approval rule: %w", err)
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (database *Database) PutApprovalRule(ctx context.Context, value protocol.ApprovalRule) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = database.database.ExecContext(ctx, `
		INSERT INTO approval_rules (id, body, created_at) VALUES (?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET body = excluded.body`,
		value.ID, string(body), encodeTime(time.Now()))
	if err != nil {
		return fmt.Errorf("sqlite: put approval rule: %w", err)
	}
	return nil
}

func (database *Database) DeleteApprovalRule(ctx context.Context, id string) error {
	result, err := database.database.ExecContext(ctx, `DELETE FROM approval_rules WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("sqlite: delete approval rule: %w", err)
	}
	changed, _ := result.RowsAffected()
	if changed == 0 {
		return settings.ErrNotFound
	}
	return nil
}

func (database *Database) ListSchedules(ctx context.Context) ([]protocol.Schedule, error) {
	rows, err := database.database.QueryContext(ctx, `SELECT body FROM schedules ORDER BY updated_at DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list schedules: %w", err)
	}
	defer rows.Close()
	values := make([]protocol.Schedule, 0)
	for rows.Next() {
		var body string
		if err := rows.Scan(&body); err != nil {
			return nil, err
		}
		var value protocol.Schedule
		if err := json.Unmarshal([]byte(body), &value); err != nil {
			return nil, fmt.Errorf("sqlite: decode schedule: %w", err)
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (database *Database) GetSchedule(ctx context.Context, id string) (protocol.Schedule, error) {
	var body string
	err := database.database.QueryRowContext(ctx, `SELECT body FROM schedules WHERE id = ?`, id).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return protocol.Schedule{}, settings.ErrNotFound
	}
	if err != nil {
		return protocol.Schedule{}, fmt.Errorf("sqlite: get schedule: %w", err)
	}
	var value protocol.Schedule
	if err := json.Unmarshal([]byte(body), &value); err != nil {
		return protocol.Schedule{}, fmt.Errorf("sqlite: decode schedule: %w", err)
	}
	return value, nil
}

func (database *Database) PutSchedule(ctx context.Context, value protocol.Schedule, expectedRevision *uint64) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if expectedRevision == nil {
		_, err = database.database.ExecContext(ctx, `
			INSERT INTO schedules (id, body, updated_at) VALUES (?, ?, ?)`,
			value.ID, string(body), encodeTime(time.Now()))
	} else {
		result, updateErr := database.database.ExecContext(ctx, `
			UPDATE schedules SET body = ?, updated_at = ?
			WHERE id = ? AND json_extract(body, '$.revision') = ?`,
			string(body), encodeTime(time.Now()), value.ID, *expectedRevision)
		err = updateErr
		if err == nil {
			changed, _ := result.RowsAffected()
			if changed == 0 {
				return protocol.ErrRevisionConflict
			}
		}
	}
	if err != nil {
		return fmt.Errorf("sqlite: put schedule %s: %w", value.ID, err)
	}
	return nil
}

func (database *Database) DeleteSchedule(ctx context.Context, id string) error {
	result, err := database.database.ExecContext(ctx, `DELETE FROM schedules WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("sqlite: delete schedule: %w", err)
	}
	changed, _ := result.RowsAffected()
	if changed == 0 {
		return settings.ErrNotFound
	}
	return nil
}
