package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/Tangerg/scope/app/runtime/internal/domain/goal"
	"github.com/Tangerg/scope/app/runtime/internal/domain/modelref"
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
func (g *GoalStore) Get(ctx context.Context, sessionID string) (goal.Goal, bool, error) {
	row := conn(ctx, g.db).QueryRowContext(ctx,
		`SELECT session_id, objective, status, reason_code, reason_detail, provider, model, reasoning_effort, capabilities, budget, used, incarnation_id, revision, created_at, updated_at
		 FROM goals WHERE session_id = ?`, sessionID)
	loaded, err := scanGoal(row)
	if errors.Is(err, sql.ErrNoRows) {
		return goal.Goal{}, false, nil
	}
	if err != nil {
		return goal.Goal{}, false, err
	}
	return loaded, true, nil
}

// Save is the goal CAS and the sole authority that advances revisions.
// INSERT-if-absent (not INSERT OR REPLACE) is deliberate — a stale writer whose
// row was cleared must not resurrect it.
func (g *GoalStore) Save(ctx context.Context, record goal.Goal, expected goal.Version) (goal.Goal, bool, error) {
	if expected == (goal.Version{}) {
		record.Revision = 1
	} else {
		if expected.IncarnationID == "" || expected.Revision <= 0 {
			return goal.Goal{}, false, errors.New("sqlite: expected goal version is invalid")
		}
		if expected.Revision == math.MaxInt64 {
			return goal.Goal{}, false, errors.New("sqlite: goal revision exhausted")
		}
		record.Revision = expected.Revision + 1
	}
	if err := record.ValidateSnapshot(); err != nil {
		return goal.Goal{}, false, fmt.Errorf("sqlite: validate goal: %w", err)
	}
	budget, err := json.Marshal(goalBudget{MaxRuns: record.Budget.MaxRuns, MaxCostUSD: record.Budget.MaxCostUSD, MaxSteps: record.Budget.MaxSteps})
	if err != nil {
		return goal.Goal{}, false, fmt.Errorf("sqlite: encode goal budget: %w", err)
	}
	used, err := json.Marshal(goalUsed{Runs: record.Used.Runs, CostUSD: record.Used.CostUSD, Steps: record.Used.Steps})
	if err != nil {
		return goal.Goal{}, false, fmt.Errorf("sqlite: encode goal used: %w", err)
	}
	capabilities, err := encodeRunCapabilities(record.Capabilities)
	if err != nil {
		return goal.Goal{}, false, fmt.Errorf("sqlite: encode goal capabilities: %w", err)
	}
	if expected == (goal.Version{}) {
		res, execContextErr := conn(ctx, g.db).ExecContext(ctx,
			`INSERT INTO goals(session_id, objective, status, reason_code, reason_detail, provider, model, reasoning_effort, capabilities, budget, used, incarnation_id, revision, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(session_id) DO NOTHING`,
			record.SessionID, record.Objective, string(record.Status), string(record.Reason.Code), record.Reason.Detail, record.ModelSelection.Provider(), record.ModelSelection.Model(), record.ModelSelection.ReasoningEffort(),
			capabilities, string(budget), string(used), record.IncarnationID, record.Revision, record.CreatedAt.UTC().UnixNano(), record.UpdatedAt.UTC().UnixNano())
		if execContextErr != nil {
			return goal.Goal{}, false, fmt.Errorf("sqlite: insert goal: %w", execContextErr)
		}
		applied, execContextErr := rowsAffected(res)
		if execContextErr != nil || !applied {
			return goal.Goal{}, applied, execContextErr
		}
		return record, true, nil
	}
	res, err := conn(ctx, g.db).ExecContext(ctx,
		`UPDATE goals SET objective = ?, status = ?, reason_code = ?, reason_detail = ?, provider = ?, model = ?, reasoning_effort = ?, capabilities = ?, budget = ?, used = ?, incarnation_id = ?, revision = ?, created_at = ?, updated_at = ?
		 WHERE session_id = ? AND incarnation_id = ? AND revision = ?`,
		record.Objective, string(record.Status), string(record.Reason.Code), record.Reason.Detail, record.ModelSelection.Provider(), record.ModelSelection.Model(), record.ModelSelection.ReasoningEffort(),
		capabilities, string(budget), string(used), record.IncarnationID, record.Revision, record.CreatedAt.UTC().UnixNano(), record.UpdatedAt.UTC().UnixNano(),
		record.SessionID, expected.IncarnationID, expected.Revision)
	if err != nil {
		return goal.Goal{}, false, fmt.Errorf("sqlite: save goal: %w", err)
	}
	applied, err := rowsAffected(res)
	if err != nil || !applied {
		return goal.Goal{}, applied, err
	}
	return record, true, nil
}

// RecordRun records a terminal goal-owned Run and applies its aggregate
// accounting in one transaction. goal_runs is an immutable idempotency ledger:
// a repeated terminal delivery for the same Run cannot charge the Goal twice,
// while an older incarnation is retained as history but never mutates a newer Goal.
func (g *GoalStore) RecordRun(ctx context.Context, record goal.RunRecord) error {
	if err := record.Validate(); err != nil {
		return fmt.Errorf("sqlite: record Goal Run: %w", err)
	}
	return RunInTx(ctx, g.db, func(ctx context.Context) error {
		res, err := conn(ctx, g.db).ExecContext(ctx,
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
			return g.validateExistingRun(ctx, record)
		}

		existing, found, err := g.Get(ctx, record.SessionID)
		if err != nil {
			return err
		}
		if !found || existing.IncarnationID != record.IncarnationID {
			return nil
		}
		expected := existing.Version()
		existing.RecordRun(record)
		_, applied, err := g.Save(ctx, existing, expected)
		if err != nil {
			return err
		}
		if !applied {
			return errors.New("sqlite: record Goal Run lost Goal ownership")
		}
		return nil
	})
}

func (g *GoalStore) validateExistingRun(ctx context.Context, record goal.RunRecord) error {
	var (
		sessionID     string
		incarnationID string
		outcome       string
		costUSD       float64
		steps         int
		completedAt   int64
	)
	err := conn(ctx, g.db).QueryRowContext(ctx,
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
func (g *GoalStore) Clear(ctx context.Context, sessionID string) error {
	if _, err := conn(ctx, g.db).ExecContext(ctx, `DELETE FROM goals WHERE session_id = ?`, sessionID); err != nil {
		return fmt.Errorf("sqlite: clear goal: %w", err)
	}
	return nil
}

// ClearIf removes the session's goal only when its version matches expected
// (the loop's CAS delete), reporting whether it applied.
func (g *GoalStore) ClearIf(ctx context.Context, sessionID string, expected goal.Version) (bool, error) {
	res, err := conn(ctx, g.db).ExecContext(ctx,
		`DELETE FROM goals WHERE session_id = ? AND incarnation_id = ? AND revision = ?`, sessionID, expected.IncarnationID, expected.Revision)
	if err != nil {
		return false, fmt.Errorf("sqlite: clear goal (cas): %w", err)
	}
	return rowsAffected(res)
}

// List returns every stored goal (for the boot reconcile).
func (g *GoalStore) List(ctx context.Context) ([]goal.Goal, error) {
	rows, err := conn(ctx, g.db).QueryContext(ctx,
		`SELECT session_id, objective, status, reason_code, reason_detail, provider, model, reasoning_effort, capabilities, budget, used, incarnation_id, revision, created_at, updated_at FROM goals`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list goals: %w", err)
	}
	defer rows.Close()
	var out []goal.Goal
	for rows.Next() {
		loaded, err := scanGoal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, loaded)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list goals: %w", err)
	}
	return out, nil
}

// scanGoal decodes one row of the goals table. Both queries select the same
// fourteen columns in the same order (session_id first), so [scanRow] covers
// *sql.Row (Get) and *sql.Rows (List) alike.
func scanGoal(row scanRow) (goal.Goal, error) {
	var (
		g                                goal.Goal
		status                           string
		reasonCode                       string
		provider, model, reasoningEffort string
		capabilitiesJSON                 string
		budgetJSON, usedJSON             string
		createdAt, updatedAt             int64
	)
	if err := row.Scan(&g.SessionID, &g.Objective, &status, &reasonCode, &g.Reason.Detail, &provider, &model, &reasoningEffort, &capabilitiesJSON, &budgetJSON, &usedJSON, &g.IncarnationID, &g.Revision, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return goal.Goal{}, err
		}
		return goal.Goal{}, fmt.Errorf("sqlite: scan goal: %w", err)
	}
	selection, err := modelref.NewWithReasoningEffort(provider, model, reasoningEffort)
	if err != nil {
		return goal.Goal{}, fmt.Errorf("sqlite: decode goal model selection: %w", err)
	}
	g.ModelSelection = selection
	capabilities, err := decodeRunCapabilities(capabilitiesJSON)
	if err != nil {
		return goal.Goal{}, fmt.Errorf("sqlite: decode goal capabilities: %w", err)
	}
	g.Capabilities = capabilities
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
