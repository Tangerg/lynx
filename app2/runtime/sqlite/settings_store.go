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
