package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	plandomain "github.com/Tangerg/lynx/app2/runtime/domain/plan"
)

type planBody struct {
	Steps []planStepBody `json:"steps"`
}
type planStepBody struct {
	Description string `json:"description"`
	Status      string `json:"status"`
}

func (database *Database) LoadPlan(ctx context.Context, sessionID string) (plandomain.State, error) {
	value, err := scanPlan(database.database.QueryRowContext(ctx, `SELECT session_id,revision,body,updated_at FROM plans WHERE session_id=?`, sessionID))
	if !errors.Is(err, plandomain.ErrNotFound) {
		return value, err
	}
	var exists int
	if lookupErr := database.database.QueryRowContext(ctx, `SELECT 1 FROM sessions WHERE id=?`, sessionID).Scan(&exists); errors.Is(lookupErr, sql.ErrNoRows) {
		return plandomain.State{}, plandomain.ErrNotFound
	} else if lookupErr != nil {
		return plandomain.State{}, lookupErr
	}
	return plandomain.New(sessionID)
}

func (database *Database) SavePlan(ctx context.Context, value plandomain.State, expectedRevision uint64) error {
	return savePlanCAS(ctx, database.database, value, expectedRevision)
}

type planExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func savePlanCAS(ctx context.Context, execer planExecer, value plandomain.State, expectedRevision uint64) error {
	body, err := encodePlanBody(value)
	if err != nil {
		return err
	}
	var result sql.Result
	if expectedRevision == 0 {
		result, err = execer.ExecContext(ctx, `INSERT INTO plans(session_id,revision,body,updated_at) VALUES(?,?,?,?) ON CONFLICT(session_id) DO NOTHING`,
			value.SessionID(), value.Revision(), body, encodeTime(value.UpdatedAt()))
	} else {
		result, err = execer.ExecContext(ctx, `UPDATE plans SET revision=?,body=?,updated_at=? WHERE session_id=? AND revision=?`,
			value.Revision(), body, encodeTime(value.UpdatedAt()), value.SessionID(), expectedRevision)
	}
	if err != nil {
		return fmt.Errorf("sqlite: save Plan: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return plandomain.ErrVersionConflict
	}
	return nil
}

func (database *Database) EnterPlanMode(ctx context.Context, sessionID string, now time.Time) (bool, error) {
	result, err := database.database.ExecContext(ctx, `INSERT INTO plan_modes(session_id,entered_at) VALUES(?,?) ON CONFLICT(session_id) DO NOTHING`, sessionID, encodeTime(now.UTC()))
	if err != nil {
		return false, fmt.Errorf("sqlite: enter Plan mode: %w", err)
	}
	changed, err := result.RowsAffected()
	return changed == 1, err
}

func (database *Database) ExitPlanMode(ctx context.Context, sessionID string) (bool, error) {
	result, err := database.database.ExecContext(ctx, `DELETE FROM plan_modes WHERE session_id=?`, sessionID)
	if err != nil {
		return false, fmt.Errorf("sqlite: exit Plan mode: %w", err)
	}
	changed, err := result.RowsAffected()
	return changed == 1, err
}

func (database *Database) IsPlanMode(ctx context.Context, sessionID string) (bool, error) {
	var exists int
	err := database.database.QueryRowContext(ctx, `SELECT 1 FROM plan_modes WHERE session_id=?`, sessionID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

type planScanner interface{ Scan(...any) error }

func scanPlan(row planScanner) (plandomain.State, error) {
	var sessionID, body, updated string
	var revision uint64
	if err := row.Scan(&sessionID, &revision, &body, &updated); errors.Is(err, sql.ErrNoRows) {
		return plandomain.State{}, plandomain.ErrNotFound
	} else if err != nil {
		return plandomain.State{}, fmt.Errorf("sqlite: scan Plan: %w", err)
	}
	var stored planBody
	if err := json.Unmarshal([]byte(body), &stored); err != nil {
		return plandomain.State{}, fmt.Errorf("sqlite: decode Plan: %w", err)
	}
	steps := planSteps(stored)
	updatedAt, err := decodeTime(updated)
	if err != nil {
		return plandomain.State{}, err
	}
	return plandomain.Rehydrate(plandomain.Restore{SessionID: sessionID, Steps: steps, Revision: revision, UpdatedAt: updatedAt})
}

func decodePlanBoundary(body string) (plandomain.Boundary, error) {
	var stored planBody
	if err := json.Unmarshal([]byte(body), &stored); err != nil {
		return plandomain.Boundary{}, fmt.Errorf("sqlite: decode Plan boundary: %w", err)
	}
	value, err := plandomain.NewBoundary(planSteps(stored))
	if err != nil {
		return plandomain.Boundary{}, fmt.Errorf("sqlite: invalid Plan boundary: %w", err)
	}
	return value, nil
}

func planSteps(stored planBody) []plandomain.Step {
	steps := make([]plandomain.Step, len(stored.Steps))
	for index, step := range stored.Steps {
		steps[index] = plandomain.Step{Description: step.Description, Status: plandomain.Status(step.Status)}
	}
	return steps
}

func emptyPlanBody() (string, error) {
	body, err := encodePlanSteps(nil)
	if err != nil {
		return "", fmt.Errorf("sqlite: encode empty Plan boundary: %w", err)
	}
	return body, nil
}

func encodePlanBody(value plandomain.State) (string, error) {
	return encodePlanSteps(value.Steps())
}

func encodePlanBoundary(value plandomain.Boundary) (string, error) {
	return encodePlanSteps(value.Steps())
}

func encodePlanSteps(steps []plandomain.Step) (string, error) {
	stored := make([]planStepBody, len(steps))
	for index, step := range steps {
		stored[index] = planStepBody{Description: step.Description, Status: string(step.Status)}
	}
	body, err := json.Marshal(planBody{Steps: stored})
	if err != nil {
		return "", fmt.Errorf("sqlite: encode Plan: %w", err)
	}
	return string(body), nil
}
