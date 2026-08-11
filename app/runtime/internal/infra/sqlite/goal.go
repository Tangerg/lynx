package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
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
	MaxRuns    int     `json:"max_runs"`
	MaxCostUSD float64 `json:"max_cost_usd"`
	MaxSteps   int     `json:"max_steps"`
}

type goalUsed struct {
	Runs    int     `json:"runs"`
	CostUSD float64 `json:"cost_usd"`
	Steps   int     `json:"steps"`
}

// Get returns the session's goal, or (zero, false, nil) when it has none.
func (s *GoalStore) Get(ctx context.Context, sessionID string) (goal.Goal, bool, error) {
	row := conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT session_id, objective, status, reason_code, reason_detail, provider, model, budget, used, incarnation_id, revision, created_at, updated_at
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

// Save is the goal CAS and the sole authority that advances revisions.
// INSERT-if-absent (not INSERT OR REPLACE) is deliberate — a stale writer whose
// row was cleared must not resurrect it.
func (s *GoalStore) Save(ctx context.Context, g goal.Goal, expected goal.Version) (goal.Goal, bool, error) {
	if expected == (goal.Version{}) {
		g.Revision = 1
	} else {
		if expected.IncarnationID == "" || expected.Revision <= 0 {
			return goal.Goal{}, false, errors.New("sqlite: expected goal version is invalid")
		}
		if expected.Revision == math.MaxInt64 {
			return goal.Goal{}, false, errors.New("sqlite: goal revision exhausted")
		}
		g.Revision = expected.Revision + 1
	}
	if err := g.ValidateSnapshot(); err != nil {
		return goal.Goal{}, false, fmt.Errorf("sqlite: validate goal: %w", err)
	}
	budget, err := json.Marshal(goalBudget{MaxRuns: g.Budget.MaxRuns, MaxCostUSD: g.Budget.MaxCostUSD, MaxSteps: g.Budget.MaxSteps})
	if err != nil {
		return goal.Goal{}, false, fmt.Errorf("sqlite: encode goal budget: %w", err)
	}
	used, err := json.Marshal(goalUsed{Runs: g.Used.Runs, CostUSD: g.Used.CostUSD, Steps: g.Used.Steps})
	if err != nil {
		return goal.Goal{}, false, fmt.Errorf("sqlite: encode goal used: %w", err)
	}
	if expected == (goal.Version{}) {
		res, err := conn(ctx, s.db).ExecContext(ctx,
			`INSERT INTO goals(session_id, objective, status, reason_code, reason_detail, provider, model, budget, used, incarnation_id, revision, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(session_id) DO NOTHING`,
			g.SessionID, g.Objective, string(g.Status), string(g.Reason.Code), g.Reason.Detail, g.ModelSelection.Provider(), g.ModelSelection.Model(),
			string(budget), string(used), g.IncarnationID, g.Revision, g.CreatedAt.UTC().UnixNano(), g.UpdatedAt.UTC().UnixNano())
		if err != nil {
			return goal.Goal{}, false, fmt.Errorf("sqlite: insert goal: %w", err)
		}
		applied, err := rowsAffected(res)
		if err != nil || !applied {
			return goal.Goal{}, applied, err
		}
		return g, true, nil
	}
	res, err := conn(ctx, s.db).ExecContext(ctx,
		`UPDATE goals SET objective = ?, status = ?, reason_code = ?, reason_detail = ?, provider = ?, model = ?, budget = ?, used = ?, incarnation_id = ?, revision = ?, created_at = ?, updated_at = ?
		 WHERE session_id = ? AND incarnation_id = ? AND revision = ?`,
		g.Objective, string(g.Status), string(g.Reason.Code), g.Reason.Detail, g.ModelSelection.Provider(), g.ModelSelection.Model(),
		string(budget), string(used), g.IncarnationID, g.Revision, g.CreatedAt.UTC().UnixNano(), g.UpdatedAt.UTC().UnixNano(),
		g.SessionID, expected.IncarnationID, expected.Revision)
	if err != nil {
		return goal.Goal{}, false, fmt.Errorf("sqlite: save goal: %w", err)
	}
	applied, err := rowsAffected(res)
	if err != nil || !applied {
		return goal.Goal{}, applied, err
	}
	return g, true, nil
}

// RecordRun records a terminal goal-owned Run and applies its aggregate
// accounting in one transaction. goal_runs is an immutable idempotency ledger:
// a repeated terminal delivery for the same Run cannot charge the Goal twice,
// while an older incarnation is retained as history but never mutates a newer Goal.
func (s *GoalStore) RecordRun(ctx context.Context, record goal.RunRecord) error {
	if err := record.Validate(); err != nil {
		return fmt.Errorf("sqlite: record Goal Run: %w", err)
	}
	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		res, err := conn(ctx, s.db).ExecContext(ctx,
			`INSERT INTO goal_runs(run_id, session_id, incarnation_id, outcome, cost_usd, steps, completed_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(run_id) DO NOTHING`,
			record.RunID, record.SessionID, record.IncarnationID, record.Outcome.String(), record.CostUSD, record.Steps, record.CompletedAt.UTC().UnixNano())
		if err != nil {
			return fmt.Errorf("sqlite: record Goal Run: %w", err)
		}
		inserted, err := rowsAffected(res)
		if err != nil {
			return err
		}
		if !inserted {
			return s.validateExistingRun(ctx, record)
		}

		g, found, err := s.Get(ctx, record.SessionID)
		if err != nil {
			return err
		}
		if !found || g.IncarnationID != record.IncarnationID {
			return nil
		}
		expected := g.Version()
		g.RecordRun(record)
		_, applied, err := s.Save(ctx, g, expected)
		if err != nil {
			return err
		}
		if !applied {
			return errors.New("sqlite: record Goal Run lost Goal ownership")
		}
		return nil
	})
}

func (s *GoalStore) validateExistingRun(ctx context.Context, record goal.RunRecord) error {
	var (
		sessionID     string
		incarnationID string
		outcome       string
		costUSD       float64
		steps         int
		completedAt   int64
	)
	err := conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT session_id, incarnation_id, outcome, cost_usd, steps, completed_at
		   FROM goal_runs
		  WHERE run_id = ?`,
		record.RunID,
	).Scan(&sessionID, &incarnationID, &outcome, &costUSD, &steps, &completedAt)
	if err != nil {
		return fmt.Errorf("sqlite: inspect existing Goal Run %q: %w", record.RunID, err)
	}
	if sessionID == record.SessionID && incarnationID == record.IncarnationID &&
		outcome == record.Outcome.String() && costUSD == record.CostUSD &&
		steps == record.Steps && completedAt == record.CompletedAt.UTC().UnixNano() {
		return nil
	}
	return fmt.Errorf(
		"%w: Run %q is already bound to a different accounting fact",
		goal.ErrRunIdentityConflict,
		record.RunID,
	)
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
		`DELETE FROM goals WHERE session_id = ? AND incarnation_id = ? AND revision = ?`, sessionID, expected.IncarnationID, expected.Revision)
	if err != nil {
		return false, fmt.Errorf("sqlite: clear goal (cas): %w", err)
	}
	return rowsAffected(res)
}

// List returns every stored goal (for the boot reconcile).
func (s *GoalStore) List(ctx context.Context) ([]goal.Goal, error) {
	rows, err := conn(ctx, s.db).QueryContext(ctx,
		`SELECT session_id, objective, status, reason_code, reason_detail, provider, model, budget, used, incarnation_id, revision, created_at, updated_at FROM goals`)
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
		reasonCode           string
		provider, model      string
		budgetJSON, usedJSON string
		createdAt, updatedAt int64
	)
	if err := row.Scan(&g.SessionID, &g.Objective, &status, &reasonCode, &g.Reason.Detail, &provider, &model, &budgetJSON, &usedJSON, &g.IncarnationID, &g.Revision, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return goal.Goal{}, err
		}
		return goal.Goal{}, fmt.Errorf("sqlite: scan goal: %w", err)
	}
	selection, err := modelref.New(provider, model)
	if err != nil {
		return goal.Goal{}, fmt.Errorf("sqlite: decode goal model selection: %w", err)
	}
	g.ModelSelection = selection
	var budget goalBudget
	if err := json.Unmarshal([]byte(budgetJSON), &budget); err != nil {
		return goal.Goal{}, fmt.Errorf("sqlite: decode goal budget: %w", err)
	}
	var used goalUsed
	if err := json.Unmarshal([]byte(usedJSON), &used); err != nil {
		return goal.Goal{}, fmt.Errorf("sqlite: decode goal used: %w", err)
	}
	g.Status = goal.Status(status)
	g.Reason.Code = goal.ReasonCode(reasonCode)
	g.Budget = goal.Budget{MaxRuns: budget.MaxRuns, MaxCostUSD: budget.MaxCostUSD, MaxSteps: budget.MaxSteps}
	g.Used = goal.Usage{Runs: used.Runs, CostUSD: used.CostUSD, Steps: used.Steps}
	g.CreatedAt = time.Unix(0, createdAt).UTC()
	g.UpdatedAt = time.Unix(0, updatedAt).UTC()
	if err := g.ValidateSnapshot(); err != nil {
		return goal.Goal{}, fmt.Errorf("sqlite: validate goal: %w", err)
	}
	return g, nil
}
