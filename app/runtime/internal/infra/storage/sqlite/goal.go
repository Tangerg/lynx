package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
)

// GoalStore is the SQLite persistence adapter for autonomous goals: one row per session, the
// budget and used accumulators JSON blobs read/written whole with the row.
//
// Safe for concurrent use; the *sql.DB serializes writes (MaxOpenConns 1, see
// [Open]).
type GoalStore struct {
	db *sql.DB
}

// NewGoalStore wires a database with the current [Open]-installed schema to the
// autonomous-goal persistence surface.
func NewGoalStore(db *sql.DB) *GoalStore { return &GoalStore{db: db} }

type goalBudget struct {
	MaxTurns   int     `json:"max_turns"`
	MaxCostUSD float64 `json:"max_cost_usd"`
	MaxSteps   int     `json:"max_steps"`
}

type goalUsed struct {
	Turns   int     `json:"turns"`
	CostUSD float64 `json:"cost_usd"`
	Steps   int     `json:"steps"`
}

// Get returns the session's goal, or (zero, false, nil) when it has none.
func (s *GoalStore) Get(ctx context.Context, sessionID string) (goal.Goal, bool, error) {
	row := conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT session_id, objective, status, reason_cause, reason_detail, provider, model, budget, used, lease_id, revision, created_at, updated_at
		 FROM goals WHERE session_id = ?`, sessionID)
	g, err := scanGoal(row)
	if errors.Is(err, sql.ErrNoRows) {
		return goal.Goal{}, false, nil
	}
	if err != nil {
		return goal.Goal{}, false, err
	}
	return g, true, nil
}

// Save is the goal CAS. INSERT-if-absent
// (not INSERT OR REPLACE) is deliberate — a stale writer whose row was cleared
// must not resurrect it.
func (s *GoalStore) Save(ctx context.Context, g goal.Goal, expected goal.Version) (bool, error) {
	if err := g.ValidateSnapshot(); err != nil {
		return false, fmt.Errorf("sqlite: validate goal: %w", err)
	}
	budget, err := json.Marshal(goalBudget{MaxTurns: g.Budget.MaxTurns, MaxCostUSD: g.Budget.MaxCostUSD, MaxSteps: g.Budget.MaxSteps})
	if err != nil {
		return false, fmt.Errorf("sqlite: encode goal budget: %w", err)
	}
	used, err := json.Marshal(goalUsed{Turns: g.Used.Turns, CostUSD: g.Used.CostUSD, Steps: g.Used.Steps})
	if err != nil {
		return false, fmt.Errorf("sqlite: encode goal used: %w", err)
	}
	if expected == (goal.Version{}) {
		res, err := conn(ctx, s.db).ExecContext(ctx,
			`INSERT INTO goals(session_id, objective, status, reason_cause, reason_detail, provider, model, budget, used, lease_id, revision, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(session_id) DO NOTHING`,
			g.SessionID, g.Objective, string(g.Status), int(g.Reason.Cause), g.Reason.Detail, g.Provider, g.Model,
			string(budget), string(used), g.LeaseID, g.Revision, g.CreatedAt.UTC().UnixNano(), g.UpdatedAt.UTC().UnixNano())
		if err != nil {
			return false, fmt.Errorf("sqlite: insert goal: %w", err)
		}
		return rowsAffected(res)
	}
	res, err := conn(ctx, s.db).ExecContext(ctx,
		`UPDATE goals SET objective = ?, status = ?, reason_cause = ?, reason_detail = ?, provider = ?, model = ?, budget = ?, used = ?, lease_id = ?, revision = ?, created_at = ?, updated_at = ?
		 WHERE session_id = ? AND lease_id = ? AND revision = ?`,
		g.Objective, string(g.Status), int(g.Reason.Cause), g.Reason.Detail, g.Provider, g.Model,
		string(budget), string(used), g.LeaseID, g.Revision, g.CreatedAt.UTC().UnixNano(), g.UpdatedAt.UTC().UnixNano(),
		g.SessionID, expected.LeaseID, expected.Revision)
	if err != nil {
		return false, fmt.Errorf("sqlite: save goal: %w", err)
	}
	return rowsAffected(res)
}

func rowsAffected(res sql.Result) (bool, error) {
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("sqlite: goal rows affected: %w", err)
	}
	return n == 1, nil
}

// Clear removes the session's goal unconditionally; a missing goal is not an
// error.
func (s *GoalStore) Clear(ctx context.Context, sessionID string) error {
	if _, err := conn(ctx, s.db).ExecContext(ctx, `DELETE FROM goals WHERE session_id = ?`, sessionID); err != nil {
		return fmt.Errorf("sqlite: clear goal: %w", err)
	}
	return nil
}

// ClearIf removes the session's goal only when its version matches expected
// (the loop's CAS delete), reporting whether it applied.
func (s *GoalStore) ClearIf(ctx context.Context, sessionID string, expected goal.Version) (bool, error) {
	res, err := conn(ctx, s.db).ExecContext(ctx,
		`DELETE FROM goals WHERE session_id = ? AND lease_id = ? AND revision = ?`, sessionID, expected.LeaseID, expected.Revision)
	if err != nil {
		return false, fmt.Errorf("sqlite: clear goal (cas): %w", err)
	}
	return rowsAffected(res)
}

// List returns every stored goal (for the boot reconcile).
func (s *GoalStore) List(ctx context.Context) ([]goal.Goal, error) {
	rows, err := conn(ctx, s.db).QueryContext(ctx,
		`SELECT session_id, objective, status, reason_cause, reason_detail, provider, model, budget, used, lease_id, revision, created_at, updated_at FROM goals`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list goals: %w", err)
	}
	defer rows.Close()
	var out []goal.Goal
	for rows.Next() {
		g, err := scanGoal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list goals: %w", err)
	}
	return out, nil
}

// scanGoal decodes one row of the goals table. Both queries select the same
// thirteen columns in the same order (session_id first), so [scanRow] covers
// *sql.Row (Get) and *sql.Rows (List) alike.
func scanGoal(row scanRow) (goal.Goal, error) {
	var (
		g                    goal.Goal
		status               string
		reasonCause          int
		budgetJSON, usedJSON string
		createdAt, updatedAt int64
	)
	if err := row.Scan(&g.SessionID, &g.Objective, &status, &reasonCause, &g.Reason.Detail, &g.Provider, &g.Model, &budgetJSON, &usedJSON, &g.LeaseID, &g.Revision, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return goal.Goal{}, err
		}
		return goal.Goal{}, fmt.Errorf("sqlite: scan goal: %w", err)
	}
	var budget goalBudget
	if err := json.Unmarshal([]byte(budgetJSON), &budget); err != nil {
		return goal.Goal{}, fmt.Errorf("sqlite: decode goal budget: %w", err)
	}
	var used goalUsed
	if err := json.Unmarshal([]byte(usedJSON), &used); err != nil {
		return goal.Goal{}, fmt.Errorf("sqlite: decode goal used: %w", err)
	}
	g.Status = goal.Status(status)
	g.Reason.Cause = goal.ReasonCause(reasonCause)
	g.Budget = goal.Budget{MaxTurns: budget.MaxTurns, MaxCostUSD: budget.MaxCostUSD, MaxSteps: budget.MaxSteps}
	g.Used = goal.Usage{Turns: used.Turns, CostUSD: used.CostUSD, Steps: used.Steps}
	g.CreatedAt = time.Unix(0, createdAt).UTC()
	g.UpdatedAt = time.Unix(0, updatedAt).UTC()
	if err := g.ValidateSnapshot(); err != nil {
		return goal.Goal{}, fmt.Errorf("sqlite: validate goal: %w", err)
	}
	return g, nil
}
