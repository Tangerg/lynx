package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/lynx/app2/runtime/runflow"
)

func (database *Database) ListInterruptSets(ctx context.Context, sessionID, rootRunID string) ([]protocol.PendingInterruptSet, error) {
	query := `SELECT i.body FROM interrupt_sets i`
	arguments := make([]any, 0, 2)
	if rootRunID != "" {
		query += ` JOIN runs r ON r.id = i.run_id WHERE r.id = ? AND r.parent_run_id IS NULL`
		arguments = append(arguments, rootRunID)
	} else if sessionID != "" {
		query += ` WHERE i.session_id = ?`
		arguments = append(arguments, sessionID)
	}
	query += ` ORDER BY i.created_at, i.run_id`
	rows, err := database.database.QueryContext(ctx, query, arguments...)
	if err != nil { return nil, fmt.Errorf("sqlite: list interrupt sets: %w", err) }
	defer rows.Close()
	values := make([]protocol.PendingInterruptSet, 0)
	for rows.Next() {
		var body string
		if err := rows.Scan(&body); err != nil { return nil, err }
		var value protocol.PendingInterruptSet
		if err := json.Unmarshal([]byte(body), &value); err != nil { return nil, fmt.Errorf("sqlite: decode interrupt set: %w", err) }
		values = append(values, value)
	}
	return values, rows.Err()
}

func (database *Database) PutInterruptSet(ctx context.Context, runID, sessionID string, value protocol.PendingInterruptSet) error {
	body, err := json.Marshal(value)
	if err != nil { return err }
	now := encodeTime(time.Now())
	_, err = database.database.ExecContext(ctx, `INSERT INTO interrupt_sets(run_id,session_id,body,created_at,updated_at) VALUES(?,?,?,?,?) ON CONFLICT(run_id) DO UPDATE SET body=excluded.body,updated_at=excluded.updated_at`, runID, sessionID, string(body), now, now)
	return err
}

func (database *Database) GetInterruptSet(ctx context.Context, runID string) (protocol.PendingInterruptSet, error) {
	var body string
	err := database.database.QueryRowContext(ctx, `SELECT body FROM interrupt_sets WHERE run_id=?`, runID).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) { return protocol.PendingInterruptSet{}, runflow.ErrInterruptSetNotFound }
	if err != nil { return protocol.PendingInterruptSet{}, fmt.Errorf("sqlite: get interrupt set: %w", err) }
	var value protocol.PendingInterruptSet
	if err := json.Unmarshal([]byte(body), &value); err != nil { return protocol.PendingInterruptSet{}, fmt.Errorf("sqlite: decode interrupt set: %w", err) }
	return value, nil
}

func (database *Database) DeleteInterruptSet(ctx context.Context, runID string) error {
	_, err := database.database.ExecContext(ctx, `DELETE FROM interrupt_sets WHERE run_id=?`, runID)
	return err
}
