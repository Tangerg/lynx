package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	conversationdomain "github.com/Tangerg/lynx/app2/runtime/domain/conversation"
	goaldomain "github.com/Tangerg/lynx/app2/runtime/domain/goal"
	plandomain "github.com/Tangerg/lynx/app2/runtime/domain/plan"
	"github.com/Tangerg/lynx/app2/runtime/domain/session"
	"github.com/Tangerg/lynx/app2/runtime/domain/transcript"
	"github.com/Tangerg/lynx/app2/runtime/domain/toolresult"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/lynx/app2/runtime/sessionflow"
)

const runSelect = `SELECT id, session_id, coalesce(parent_run_id, ''),
	coalesce(root_run_id, ''), coalesce(spawned_by_item_id, ''), status,
	coalesce(active_segment_id, ''), provider, model, coalesce(outcome, ''),
	detail, body, created_at, updated_at, coalesce(finished_at, '') FROM runs`

func (database *Database) ReadSessionMaterial(
	ctx context.Context,
	id session.ID,
) (sessionflow.Material, error) {
	transaction, err := database.database.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return sessionflow.Material{}, err
	}
	defer transaction.Rollback()

	value, err := scanSession(transaction.QueryRowContext(ctx, `
		SELECT id,title,workspace_path,provider,model,favorite,revision,created_at,updated_at
		FROM sessions WHERE id=?`, id.String()))
	if err != nil {
		return sessionflow.Material{}, err
	}
	emptyPlan, err := plandomain.New(id.String())
	if err != nil {
		return sessionflow.Material{}, err
	}
	material := sessionflow.Material{
		Session:        value,
		Interrupts:     []protocol.PendingInterruptSet{},
		Plan:           emptyPlan,
		PlanBoundaries: make(map[string]plandomain.Boundary),
	}
	if err := readMaterialRuns(ctx, transaction, id.String(), &material); err != nil {
		return sessionflow.Material{}, err
	}
	if err := readMaterialItems(ctx, transaction, id.String(), &material); err != nil {
		return sessionflow.Material{}, err
	}
	if err := readMaterialMessages(ctx, transaction, id.String(), &material); err != nil {
		return sessionflow.Material{}, err
	}
	if err := readMaterialInterrupts(ctx, transaction, id.String(), &material); err != nil {
		return sessionflow.Material{}, err
	}
	if err := readMaterialPlan(ctx, transaction, id.String(), &material); err != nil {
		return sessionflow.Material{}, err
	}
	if err := readMaterialGoal(ctx, transaction, id.String(), &material); err != nil {
		return sessionflow.Material{}, err
	}
	if err := readMaterialToolResults(ctx, transaction, id.String(), &material); err != nil {
		return sessionflow.Material{}, err
	}
	if err := transaction.Commit(); err != nil {
		return sessionflow.Material{}, err
	}
	return material, nil
}

func readMaterialRuns(
	ctx context.Context,
	transaction *sql.Tx,
	sessionID string,
	material *sessionflow.Material,
) error {
	rows, err := transaction.QueryContext(
		ctx,
		runSelect+` WHERE session_id=? ORDER BY created_at,id`,
		sessionID,
	)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		record, err := scanRun(rows)
		if err != nil {
			return err
		}
		material.Runs = append(material.Runs, record)
	}
	return rows.Err()
}

func readMaterialItems(
	ctx context.Context,
	transaction *sql.Tx,
	sessionID string,
	material *sessionflow.Material,
) error {
	rows, err := transaction.QueryContext(ctx, `
		SELECT id,session_id,run_id,ordinal,body,search_text,created_at
		FROM items WHERE session_id=? ORDER BY created_at,run_id,ordinal`, sessionID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var record transcript.Record
		var createdAt string
		if err := rows.Scan(
			&record.ID,
			&record.SessionID,
			&record.RunID,
			&record.Ordinal,
			&record.Body,
			&record.SearchText,
			&createdAt,
		); err != nil {
			return err
		}
		record.CreatedAt, err = decodeTime(createdAt)
		if err != nil {
			return err
		}
		material.Items = append(material.Items, record)
	}
	return rows.Err()
}

func readMaterialMessages(
	ctx context.Context,
	transaction *sql.Tx,
	sessionID string,
	material *sessionflow.Material,
) error {
	rows, err := transaction.QueryContext(ctx, `
		SELECT session_id,run_id,ordinal,body
		FROM conversation_messages WHERE session_id=? ORDER BY ordinal`, sessionID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var record conversationdomain.Record
		if err := rows.Scan(
			&record.SessionID,
			&record.RunID,
			&record.Ordinal,
			&record.Body,
		); err != nil {
			return err
		}
		material.Messages = append(material.Messages, record)
	}
	return rows.Err()
}

func readMaterialInterrupts(
	ctx context.Context,
	transaction *sql.Tx,
	sessionID string,
	material *sessionflow.Material,
) error {
	rows, err := transaction.QueryContext(ctx, `
		SELECT body FROM interrupt_sets
		WHERE session_id=? ORDER BY created_at,run_id`, sessionID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var body string
		if err := rows.Scan(&body); err != nil {
			return err
		}
		var value protocol.PendingInterruptSet
		if err := json.Unmarshal([]byte(body), &value); err != nil {
			return err
		}
		material.Interrupts = append(material.Interrupts, value)
	}
	return rows.Err()
}

func readMaterialPlan(
	ctx context.Context,
	transaction *sql.Tx,
	sessionID string,
	material *sessionflow.Material,
) error {
	stored, err := scanPlan(transaction.QueryRowContext(ctx, `
		SELECT session_id,revision,body,updated_at FROM plans WHERE session_id=?`,
		sessionID,
	))
	if err == nil {
		material.Plan = stored
	} else if !errors.Is(err, plandomain.ErrNotFound) {
		return err
	}

	rows, err := transaction.QueryContext(ctx, `
		SELECT boundary.run_id,boundary.body
		FROM plan_boundaries boundary
		JOIN runs run ON run.id=boundary.run_id
		WHERE run.session_id=? ORDER BY run.created_at,run.id`, sessionID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var runID, body string
		if err := rows.Scan(&runID, &body); err != nil {
			return err
		}
		boundary, err := decodePlanBoundary(body)
		if err != nil {
			return err
		}
		material.PlanBoundaries[runID] = boundary
	}
	return rows.Err()
}

func readMaterialGoal(
	ctx context.Context,
	transaction *sql.Tx,
	sessionID string,
	material *sessionflow.Material,
) error {
	value, err := scanGoal(transaction.QueryRowContext(ctx, `
		SELECT session_id,incarnation_id,revision,status,coalesce(active_run_id,''),body
		FROM goals WHERE session_id=?`, sessionID))
	if err == nil {
		material.Goal = &value
		return nil
	}
	if errors.Is(err, goaldomain.ErrNotFound) {
		return nil
	}
	return err
}

func readMaterialToolResults(
	ctx context.Context,
	transaction *sql.Tx,
	sessionID string,
	material *sessionflow.Material,
) error {
	rows, err := transaction.QueryContext(ctx, `
		SELECT body FROM tool_results WHERE session_id=? ORDER BY created_at,id`,
		sessionID,
	)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var body string
		if err := rows.Scan(&body); err != nil {
			return err
		}
		var record toolresult.Record
		if err := json.Unmarshal([]byte(body), &record); err != nil {
			return err
		}
		if err := record.Validate(); err != nil {
			return err
		}
		material.ToolResults = append(material.ToolResults, record)
	}
	return rows.Err()
}
