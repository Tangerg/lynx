package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	goaldomain "github.com/Tangerg/lynx/app2/runtime/domain/goal"
)

type goalBody struct {
	Objective string         `json:"objective"`
	Provider  string         `json:"provider,omitempty"`
	Model     string         `json:"model,omitempty"`
	Reason    goalReasonBody `json:"reason"`
	Budget    goalBudgetBody `json:"budget"`
	Used      goalUsageBody  `json:"used"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
}

type goalReasonBody struct {
	Code   string `json:"code,omitempty"`
	Detail string `json:"detail,omitempty"`
}
type goalBudgetBody struct {
	MaxRuns    int     `json:"maxRuns,omitempty"`
	MaxCostUSD float64 `json:"maxCostUsd,omitempty"`
	MaxSteps   int     `json:"maxSteps,omitempty"`
}
type goalUsageBody struct {
	Runs    int     `json:"runs"`
	CostUSD float64 `json:"costUsd"`
	Steps   int     `json:"steps"`
}

func (database *Database) LoadGoal(ctx context.Context, sessionID string) (goaldomain.Goal, error) {
	return scanGoal(database.database.QueryRowContext(ctx, `
		SELECT session_id,incarnation_id,revision,status,coalesce(active_run_id,''),body
		FROM goals WHERE session_id=?`, sessionID))
}

func (database *Database) LoadGoalByRun(ctx context.Context, runID string) (goaldomain.Goal, error) {
	return scanGoal(database.database.QueryRowContext(ctx, `
		SELECT session_id,incarnation_id,revision,status,coalesce(active_run_id,''),body
		FROM goals WHERE active_run_id=?`, runID))
}

func (database *Database) ListGoals(ctx context.Context) ([]goaldomain.Goal, error) {
	rows, err := database.database.QueryContext(ctx, `
		SELECT session_id,incarnation_id,revision,status,coalesce(active_run_id,''),body
		FROM goals ORDER BY updated_at,session_id`)
	if err != nil { return nil, fmt.Errorf("sqlite: list goals: %w", err) }
	defer rows.Close()
	values := make([]goaldomain.Goal, 0)
	for rows.Next() {
		value, err := scanGoal(rows)
		if err != nil { return nil, err }
		values = append(values, value)
	}
	return values, rows.Err()
}

func (database *Database) CreateGoal(ctx context.Context, value goaldomain.Goal) error {
	body, err := encodeGoalBody(value)
	if err != nil { return err }
	result, err := database.database.ExecContext(ctx, `
		INSERT INTO goals(session_id,incarnation_id,revision,status,active_run_id,body,updated_at)
		VALUES(?,?,?,?,nullif(?,''),?,?) ON CONFLICT(session_id) DO NOTHING`, value.SessionID(), value.IncarnationID(), value.Revision(),
		string(value.Status()), value.ActiveRunID(), body, encodeTime(value.UpdatedAt()))
	if err != nil { return fmt.Errorf("sqlite: create goal: %w", err) }
	changed, err := result.RowsAffected()
	if err != nil { return err }
	if changed != 1 { return goaldomain.ErrVersionConflict }
	return nil
}

func (database *Database) SaveGoal(ctx context.Context, value goaldomain.Goal, expectedIncarnation string, expectedRevision uint64) (goaldomain.Goal, error) {
	persisted, err := value.WithRevision(expectedRevision + 1)
	if err != nil { return goaldomain.Goal{}, err }
	body, err := encodeGoalBody(persisted)
	if err != nil { return goaldomain.Goal{}, err }
	result, err := database.database.ExecContext(ctx, `
		UPDATE goals SET incarnation_id=?,revision=?,status=?,active_run_id=nullif(?,''),body=?,updated_at=?
		WHERE session_id=? AND incarnation_id=? AND revision=?`,
		persisted.IncarnationID(), persisted.Revision(), string(persisted.Status()), persisted.ActiveRunID(), body,
		encodeTime(persisted.UpdatedAt()), persisted.SessionID(), expectedIncarnation, expectedRevision)
	if err != nil { return goaldomain.Goal{}, fmt.Errorf("sqlite: save goal: %w", err) }
	changed, err := result.RowsAffected()
	if err != nil { return goaldomain.Goal{}, err }
	if changed != 1 { return goaldomain.Goal{}, goaldomain.ErrVersionConflict }
	return persisted, nil
}

func (database *Database) RemoveGoal(ctx context.Context, sessionID, incarnation string, revision uint64) error {
	result, err := database.database.ExecContext(ctx, `DELETE FROM goals WHERE session_id=? AND incarnation_id=? AND revision=?`, sessionID, incarnation, revision)
	if err != nil { return fmt.Errorf("sqlite: remove goal: %w", err) }
	changed, err := result.RowsAffected()
	if err != nil { return err }
	if changed != 1 { return goaldomain.ErrVersionConflict }
	return nil
}

type goalScanner interface { Scan(...any) error }

func scanGoal(row goalScanner) (goaldomain.Goal, error) {
	var sessionID, incarnationID, status, activeRunID, encoded string
	var revision uint64
	if err := row.Scan(&sessionID, &incarnationID, &revision, &status, &activeRunID, &encoded); errors.Is(err, sql.ErrNoRows) {
		return goaldomain.Goal{}, goaldomain.ErrNotFound
	} else if err != nil {
		return goaldomain.Goal{}, fmt.Errorf("sqlite: scan goal: %w", err)
	}
	var body goalBody
	if err := json.Unmarshal([]byte(encoded), &body); err != nil { return goaldomain.Goal{}, fmt.Errorf("sqlite: decode goal: %w", err) }
	return goaldomain.Rehydrate(goaldomain.Restore{
		SessionID: sessionID, IncarnationID: incarnationID, Objective: body.Objective,
		Provider: body.Provider, Model: body.Model, ActiveRunID: activeRunID,
		Status: goaldomain.Status(status),
		Reason: goaldomain.Reason{Code: goaldomain.ReasonCode(body.Reason.Code), Detail: body.Reason.Detail},
		Budget: goaldomain.Budget{MaxRuns: body.Budget.MaxRuns, MaxCostUSD: body.Budget.MaxCostUSD, MaxSteps: body.Budget.MaxSteps},
		Used: goaldomain.Usage{Runs: body.Used.Runs, CostUSD: body.Used.CostUSD, Steps: body.Used.Steps},
		Revision: revision, CreatedAt: body.CreatedAt, UpdatedAt: body.UpdatedAt,
	})
}

func encodeGoalBody(value goaldomain.Goal) (string, error) {
	reason, budget, used := value.Reason(), value.Budget(), value.Used()
	body, err := json.Marshal(goalBody{
		Objective: value.Objective(), Provider: value.Provider(), Model: value.Model(),
		Reason: goalReasonBody{Code: string(reason.Code), Detail: reason.Detail},
		Budget: goalBudgetBody{MaxRuns: budget.MaxRuns, MaxCostUSD: budget.MaxCostUSD, MaxSteps: budget.MaxSteps},
		Used: goalUsageBody{Runs: used.Runs, CostUSD: used.CostUSD, Steps: used.Steps},
		CreatedAt: value.CreatedAt(), UpdatedAt: value.UpdatedAt(),
	})
	if err != nil { return "", fmt.Errorf("sqlite: encode goal: %w", err) }
	return string(body), nil
}
