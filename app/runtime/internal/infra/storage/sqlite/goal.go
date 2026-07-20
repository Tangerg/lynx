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

// GoalStore implements goal.Store against SQLite: one row per session, the
// budget and used accumulators JSON blobs read/written whole with the row.
//
// Safe for concurrent use; the *sql.DB serializes writes (MaxOpenConns 1, see
// [Open]).
type GoalStore struct {
	db *sql.DB
}

var _ goal.Store = (*GoalStore)(nil)

// NewGoalStore wires a database with the current [Open]-installed schema to the
// goal.Store surface.
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
		`SELECT objective, status, reason, provider, model, budget, used, created_at, updated_at
		 FROM goals WHERE session_id = ?`, sessionID)
	g, err := scanGoal(sessionID, row)
	if errors.Is(err, sql.ErrNoRows) {
		return goal.Goal{}, false, nil
	}
	if err != nil {
		return goal.Goal{}, false, err
	}
	return g, true, nil
}

// Save upserts the session's goal.
func (s *GoalStore) Save(ctx context.Context, g goal.Goal) error {
	budget, err := json.Marshal(goalBudget{MaxTurns: g.Budget.MaxTurns, MaxCostUSD: g.Budget.MaxCostUSD, MaxSteps: g.Budget.MaxSteps})
	if err != nil {
		return fmt.Errorf("sqlite: encode goal budget: %w", err)
	}
	used, err := json.Marshal(goalUsed{Turns: g.Used.Turns, CostUSD: g.Used.CostUSD, Steps: g.Used.Steps})
	if err != nil {
		return fmt.Errorf("sqlite: encode goal used: %w", err)
	}
	_, err = conn(ctx, s.db).ExecContext(ctx,
		`INSERT OR REPLACE INTO goals(session_id, objective, status, reason, provider, model, budget, used, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		g.SessionID, g.Objective, string(g.Status), g.Reason, g.Provider, g.Model,
		string(budget), string(used), g.CreatedAt.UTC().UnixNano(), g.UpdatedAt.UTC().UnixNano())
	if err != nil {
		return fmt.Errorf("sqlite: save goal: %w", err)
	}
	return nil
}

// Clear removes the session's goal (completion / abandonment); a missing goal is
// not an error.
func (s *GoalStore) Clear(ctx context.Context, sessionID string) error {
	if _, err := conn(ctx, s.db).ExecContext(ctx, `DELETE FROM goals WHERE session_id = ?`, sessionID); err != nil {
		return fmt.Errorf("sqlite: clear goal: %w", err)
	}
	return nil
}

// List returns every stored goal (for the boot reconcile).
func (s *GoalStore) List(ctx context.Context) ([]goal.Goal, error) {
	rows, err := conn(ctx, s.db).QueryContext(ctx,
		`SELECT session_id, objective, status, reason, provider, model, budget, used, created_at, updated_at FROM goals`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list goals: %w", err)
	}
	defer rows.Close()
	var out []goal.Goal
	for rows.Next() {
		var sessionID string
		g, err := scanGoalRow(rows, &sessionID)
		if err != nil {
			return nil, err
		}
		g.SessionID = sessionID
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list goals: %w", err)
	}
	return out, nil
}

// scanner abstracts *sql.Row and *sql.Rows for the shared column decode.
type scanner interface {
	Scan(dest ...any) error
}

func scanGoal(sessionID string, row scanner) (goal.Goal, error) {
	g, err := scanGoalRow(row, nil)
	if err != nil {
		return goal.Goal{}, err
	}
	g.SessionID = sessionID
	return g, nil
}

// scanGoalRow decodes the goal columns. When sessionID is non-nil the first
// column (session_id) is scanned into it (the List query); otherwise the caller
// supplied the session id out of band (the Get query, keyed by it).
func scanGoalRow(row scanner, sessionID *string) (goal.Goal, error) {
	var (
		g                    goal.Goal
		status               string
		budgetJSON, usedJSON string
		createdAt, updatedAt int64
		dest                 []any
	)
	if sessionID != nil {
		dest = append(dest, sessionID)
	}
	dest = append(dest, &g.Objective, &status, &g.Reason, &g.Provider, &g.Model, &budgetJSON, &usedJSON, &createdAt, &updatedAt)
	if err := row.Scan(dest...); err != nil {
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
	g.Budget = goal.Budget{MaxTurns: budget.MaxTurns, MaxCostUSD: budget.MaxCostUSD, MaxSteps: budget.MaxSteps}
	g.Used = goal.Usage{Turns: used.Turns, CostUSD: used.CostUSD, Steps: used.Steps}
	g.CreatedAt = time.Unix(0, createdAt).UTC()
	g.UpdatedAt = time.Unix(0, updatedAt).UTC()
	return g, nil
}
