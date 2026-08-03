package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
)

// PlanStore persists one complete ordered Plan per session. Plans are replaced
// wholesale, so one JSON value and one monotonic revision are the entire latest
// projection; Run boundaries retain the historical values needed by fork and
// rollback.
type PlanStore struct{ db *sql.DB }

type planStepRow struct {
	Description string      `json:"description"`
	Status      plan.Status `json:"status"`
}

func NewPlanStore(db *sql.DB) *PlanStore { return &PlanStore{db: db} }

func (s *PlanStore) List(ctx context.Context, sessionID string) ([]plan.Step, error) {
	state, err := s.State(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return state.Steps, nil
}

// State returns the zero state when no Plan has been set for the session.
func (s *PlanStore) State(ctx context.Context, sessionID string) (plan.State, error) {
	var (
		stepsJSON string
		revision  uint64
		updatedNs int64
	)
	err := conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT steps, revision, updated_at FROM session_plans WHERE session_id = ?`, sessionID,
	).Scan(&stepsJSON, &revision, &updatedNs)
	if errors.Is(err, sql.ErrNoRows) {
		return plan.State{}, nil
	}
	if err != nil {
		return plan.State{}, fmt.Errorf("sqlite: read Plan: %w", err)
	}
	steps, err := decodePlanSteps(stepsJSON)
	if err != nil {
		return plan.State{}, err
	}
	return plan.State{
		Steps: steps, Revision: revision, UpdatedAt: time.Unix(0, updatedNs).UTC(),
	}, nil
}

func decodePlanSteps(stepsJSON string) ([]plan.Step, error) {
	if stepsJSON == "" {
		return nil, nil
	}
	var rows []planStepRow
	if err := json.Unmarshal([]byte(stepsJSON), &rows); err != nil {
		return nil, fmt.Errorf("sqlite: decode Plan: %w", err)
	}
	steps := make([]plan.Step, len(rows))
	for index, row := range rows {
		steps[index] = plan.Step{Description: row.Description, Status: row.Status}
	}
	if err := plan.Validate(steps); err != nil {
		return nil, fmt.Errorf("sqlite: validate Plan: %w", err)
	}
	return steps, nil
}

func (s *PlanStore) Replace(ctx context.Context, sessionID string, steps []plan.Step) error {
	if steps == nil {
		steps = []plan.Step{}
	}
	if err := plan.Validate(steps); err != nil {
		return fmt.Errorf("sqlite: validate Plan: %w", err)
	}
	rows := make([]planStepRow, len(steps))
	for index, step := range steps {
		rows[index] = planStepRow{Description: step.Description, Status: step.Status}
	}
	data, err := json.Marshal(rows)
	if err != nil {
		return fmt.Errorf("sqlite: encode Plan: %w", err)
	}
	_, err = conn(ctx, s.db).ExecContext(ctx,
		`INSERT INTO session_plans(session_id, steps, revision, updated_at) VALUES (?, ?, 1, ?)
		 ON CONFLICT(session_id) DO UPDATE SET
		   steps = excluded.steps,
		   revision = session_plans.revision + 1,
		   updated_at = excluded.updated_at`,
		sessionID, string(data), time.Now().UTC().UnixNano(),
	)
	if err != nil {
		return fmt.Errorf("sqlite: replace Plan: %w", err)
	}
	return nil
}

// CaptureBoundary freezes the session's current Plan at one terminal Run.
func (s *PlanStore) CaptureBoundary(ctx context.Context, sessionID, runID string) error {
	if _, err := conn(ctx, s.db).ExecContext(ctx,
		`INSERT INTO plan_boundaries(run_id, steps)
		 VALUES (?, COALESCE((SELECT steps FROM session_plans WHERE session_id = ?), '[]'))`,
		runID, sessionID,
	); err != nil {
		return fmt.Errorf("sqlite: capture Plan boundary for Run %q: %w", runID, err)
	}
	return nil
}

// Boundary returns the Plan captured by runID. recorded=false means the Run
// never captured a boundary; it does not mean an empty Plan.
func (s *PlanStore) Boundary(ctx context.Context, runID string) ([]plan.Step, bool, error) {
	var stepsJSON string
	err := conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT steps FROM plan_boundaries WHERE run_id = ?`, runID,
	).Scan(&stepsJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("sqlite: read Plan boundary for Run %q: %w", runID, err)
	}
	steps, err := decodePlanSteps(stepsJSON)
	if err != nil {
		return nil, false, err
	}
	return steps, true, nil
}

func (s *PlanStore) DeleteSession(ctx context.Context, sessionID string) error {
	if _, err := conn(ctx, s.db).ExecContext(ctx,
		`DELETE FROM session_plans WHERE session_id = ?`, sessionID,
	); err != nil {
		return fmt.Errorf("sqlite: delete session Plan: %w", err)
	}
	return nil
}
