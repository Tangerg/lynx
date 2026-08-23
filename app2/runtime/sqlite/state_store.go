package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/domain/state"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/lynx/app2/runtime/runflow"
)

func (database *Database) GetPlan(ctx context.Context, sessionID string) (protocol.Plan, error) {
	var body string
	err := database.database.QueryRowContext(ctx, `SELECT body FROM plans WHERE session_id = ?`, sessionID).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		var exists int
		if lookupErr := database.database.QueryRowContext(ctx, `SELECT 1 FROM sessions WHERE id = ?`, sessionID).Scan(&exists); errors.Is(lookupErr, sql.ErrNoRows) {
			return protocol.Plan{}, state.ErrNotFound
		} else if lookupErr != nil {
			return protocol.Plan{}, lookupErr
		}
		return protocol.Plan{SessionID: sessionID, Steps: []protocol.PlanStep{}}, nil
	}
	if err != nil {
		return protocol.Plan{}, fmt.Errorf("sqlite: get plan: %w", err)
	}
	var value protocol.Plan
	if err := json.Unmarshal([]byte(body), &value); err != nil {
		return protocol.Plan{}, fmt.Errorf("sqlite: decode plan: %w", err)
	}
	return value, nil
}

func (database *Database) PutPlan(ctx context.Context, value protocol.Plan, expected uint64) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	result, err := database.database.ExecContext(ctx, `
		INSERT INTO plans (session_id, revision, body, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(session_id) DO UPDATE SET revision=excluded.revision, body=excluded.body, updated_at=excluded.updated_at
		WHERE plans.revision = ?`, value.SessionID, value.Revision, string(body), encodeTime(value.UpdatedAt), expected)
	if err != nil {
		return fmt.Errorf("sqlite: put plan: %w", err)
	}
	changed, _ := result.RowsAffected()
	if changed == 0 {
		return state.ErrConflict
	}
	return nil
}

func (database *Database) GetGoal(ctx context.Context, sessionID string) (protocol.Goal, uint64, error) {
	var body string
	var incarnation uint64
	err := database.database.QueryRowContext(ctx, `SELECT body, incarnation FROM goals WHERE session_id = ?`, sessionID).Scan(&body, &incarnation)
	if errors.Is(err, sql.ErrNoRows) {
		return protocol.Goal{}, 0, state.ErrNotFound
	}
	if err != nil {
		return protocol.Goal{}, 0, fmt.Errorf("sqlite: get goal: %w", err)
	}
	var value protocol.Goal
	if err := json.Unmarshal([]byte(body), &value); err != nil {
		return protocol.Goal{}, 0, fmt.Errorf("sqlite: decode goal: %w", err)
	}
	return value, incarnation, nil
}

func (database *Database) PutGoal(ctx context.Context, value protocol.Goal, incarnation uint64, expected *uint64) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if expected == nil {
		_, err = database.database.ExecContext(ctx, `INSERT INTO goals (session_id, incarnation, status, body, updated_at) VALUES (?, ?, ?, ?, ?)`,
			value.SessionID, incarnation, value.Status, string(body), encodeTime(value.UpdatedAt))
	} else {
		var result sql.Result
		result, err = database.database.ExecContext(ctx, `UPDATE goals SET incarnation = ?, status = ?, body = ?, updated_at = ? WHERE session_id = ? AND incarnation = ?`,
			incarnation, value.Status, string(body), encodeTime(value.UpdatedAt), value.SessionID, *expected)
		if err == nil {
			changed, _ := result.RowsAffected()
			if changed == 0 {
				return state.ErrConflict
			}
		}
	}
	if err != nil {
		return fmt.Errorf("sqlite: put goal: %w", err)
	}
	return nil
}

func (database *Database) DeleteGoal(ctx context.Context, sessionID string) error {
	result, err := database.database.ExecContext(ctx, `DELETE FROM goals WHERE session_id = ?`, sessionID)
	if err != nil {
		return fmt.Errorf("sqlite: delete goal: %w", err)
	}
	changed, _ := result.RowsAffected()
	if changed == 0 {
		return state.ErrNotFound
	}
	return nil
}

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
	if err != nil {
		return nil, fmt.Errorf("sqlite: list interrupt sets: %w", err)
	}
	defer rows.Close()
	values := make([]protocol.PendingInterruptSet, 0)
	for rows.Next() {
		var body string
		if err := rows.Scan(&body); err != nil {
			return nil, err
		}
		var value protocol.PendingInterruptSet
		if err := json.Unmarshal([]byte(body), &value); err != nil {
			return nil, fmt.Errorf("sqlite: decode interrupt set: %w", err)
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (database *Database) PutInterruptSet(ctx context.Context, runID, sessionID string, value protocol.PendingInterruptSet) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	now := encodeTime(time.Now())
	_, err = database.database.ExecContext(ctx, `
		INSERT INTO interrupt_sets (run_id, session_id, body, created_at, updated_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(run_id) DO UPDATE SET body=excluded.body, updated_at=excluded.updated_at`,
		runID, sessionID, string(body), now, now)
	return err
}

func (database *Database) GetInterruptSet(ctx context.Context, runID string) (protocol.PendingInterruptSet, error) {
	var body string
	err := database.database.QueryRowContext(ctx, `SELECT body FROM interrupt_sets WHERE run_id = ?`, runID).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return protocol.PendingInterruptSet{}, runflow.ErrInterruptSetNotFound
	}
	if err != nil {
		return protocol.PendingInterruptSet{}, fmt.Errorf("sqlite: get interrupt set: %w", err)
	}
	var value protocol.PendingInterruptSet
	if err := json.Unmarshal([]byte(body), &value); err != nil {
		return protocol.PendingInterruptSet{}, fmt.Errorf("sqlite: decode interrupt set: %w", err)
	}
	return value, nil
}

func (database *Database) DeleteInterruptSet(ctx context.Context, runID string) error {
	_, err := database.database.ExecContext(ctx, `DELETE FROM interrupt_sets WHERE run_id = ?`, runID)
	return err
}
