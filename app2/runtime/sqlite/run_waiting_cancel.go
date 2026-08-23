package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	rundomain "github.com/Tangerg/lynx/app2/runtime/domain/run"
	"github.com/Tangerg/lynx/app2/runtime/runflow"
)

func (database *Database) CommitWaitingTreeCancel(
	ctx context.Context,
	write runflow.WaitingTreeCancelWrite,
) error {
	if len(write.Runs) == 0 || write.ExpectedInterrupts.RootRunID == "" {
		return errors.New("sqlite: waiting tree cancel is incomplete")
	}
	root := write.Runs[len(write.Runs)-1]
	if root.Run.Run.ID() != write.ExpectedInterrupts.RootRunID || root.Depth != 0 ||
		root.Run.Run.ParentRunID() != "" {
		return errors.New("sqlite: waiting tree cancel root is invalid")
	}
	transaction, err := database.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: begin waiting tree cancel: %w", err)
	}
	defer transaction.Rollback()
	var pendingBody string
	err = transaction.QueryRowContext(ctx, `SELECT body FROM interrupt_sets WHERE run_id = ?`,
		write.ExpectedInterrupts.RootRunID,
	).Scan(&pendingBody)
	if errors.Is(err, sql.ErrNoRows) {
		return runflow.ErrInterruptSetNotFound
	}
	if err != nil {
		return fmt.Errorf("sqlite: read waiting tree interrupt: %w", err)
	}
	var current any
	var expected any
	if err := json.Unmarshal([]byte(pendingBody), &current); err != nil {
		return fmt.Errorf("sqlite: decode waiting tree interrupt: %w", err)
	}
	expectedBody, err := json.Marshal(write.ExpectedInterrupts)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(expectedBody, &expected); err != nil {
		return err
	}
	if !reflect.DeepEqual(current, expected) {
		return runflow.ErrInterruptSetNotFound
	}
	var openMembers int
	if err := transaction.QueryRowContext(ctx, `
		SELECT count(*) FROM runs
		WHERE status != 'finished' AND (id = ? OR root_run_id = ?)`,
		write.ExpectedInterrupts.RootRunID, write.ExpectedInterrupts.RootRunID,
	).Scan(&openMembers); err != nil {
		return fmt.Errorf("sqlite: count waiting cancel tree members: %w", err)
	}
	if openMembers != len(write.Runs) {
		return errors.New("sqlite: waiting cancel does not cover every open Run")
	}
	lastDepth := ^uint32(0)
	planned := make(map[string]bool, len(write.Runs))
	for _, member := range write.Runs {
		value := member.Run.Run
		if member.Depth > lastDepth || planned[value.ID()] ||
			(value.Status() != rundomain.Running && value.Status() != rundomain.Finished) ||
			value.SessionID() != write.ExpectedInterrupts.SessionID {
			return errors.New("sqlite: waiting cancel members are invalid or out of order")
		}
		lastDepth = member.Depth
		planned[value.ID()] = true
	}
	for _, member := range write.Runs {
		value := member.Run.Run
		if value.ID() == write.ExpectedInterrupts.RootRunID {
			if value.ParentRunID() != "" || value.RootRunID() != "" {
				return errors.New("sqlite: waiting cancel root lineage is invalid")
			}
		} else if value.RootRunID() != write.ExpectedInterrupts.RootRunID ||
			value.ParentRunID() == "" || !planned[value.ParentRunID()] {
			return errors.New("sqlite: waiting cancel child lineage is invalid")
		}
		var currentSessionID, currentParentRunID, currentRootRunID string
		if err := transaction.QueryRowContext(ctx, `
			SELECT session_id, coalesce(parent_run_id, ''), coalesce(root_run_id, '')
			FROM runs WHERE id = ?`, value.ID(),
		).Scan(&currentSessionID, &currentParentRunID, &currentRootRunID); err != nil {
			return fmt.Errorf("sqlite: read waiting cancel Run %s lineage: %w", value.ID(), err)
		}
		if currentSessionID != value.SessionID() || currentParentRunID != value.ParentRunID() ||
			currentRootRunID != value.RootRunID() {
			return errors.New("sqlite: waiting cancel durable lineage changed")
		}
	}
	known := make(map[string]bool, len(write.Runs))
	for _, member := range write.Runs {
		value := member.Run.Run
		known[value.ID()] = true
		result, err := transaction.ExecContext(ctx, `
			UPDATE runs SET status = ?, active_segment_id = nullif(?, ''),
				outcome = nullif(?, ''), detail = ?, body = ?, updated_at = ?,
				finished_at = nullif(?, '')
			WHERE id = ? AND status = 'waiting' AND active_segment_id IS NULL`,
			string(value.Status()), value.ActiveSegmentID(), string(value.Outcome()), value.Detail(),
			string(member.Run.Body), encodeTime(value.UpdatedAt()), encodeOptionalTime(value.FinishedAt()), value.ID(),
		)
		if err != nil {
			return fmt.Errorf("sqlite: continue waiting cancel Run %s: %w", value.ID(), err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return rundomain.ErrInvalidTransition
		}
	}
	for _, item := range write.Items {
		if !known[item.RunID] || item.SessionID != write.ExpectedInterrupts.SessionID {
			return errors.New("sqlite: waiting cancel Item changed ownership")
		}
		if err := putItem(ctx, transaction, item); err != nil {
			return err
		}
	}
	if err := insertRunEvents(ctx, transaction, write.Events); err != nil {
		return err
	}
	if _, err := transaction.ExecContext(ctx, `DELETE FROM interrupt_sets WHERE run_id = ?`,
		write.ExpectedInterrupts.RootRunID,
	); err != nil {
		return fmt.Errorf("sqlite: consume waiting tree interrupt: %w", err)
	}
	if _, err := transaction.ExecContext(ctx, `DELETE FROM executor_checkpoints WHERE run_id = ?`,
		write.ExpectedInterrupts.RootRunID,
	); err != nil {
		return fmt.Errorf("sqlite: consume waiting tree checkpoint: %w", err)
	}
	if root.Run.Run.Status() == rundomain.Finished {
		if err := capturePlanBoundary(
			ctx, transaction, root.Run.Run.ID(), root.Run.Run.SessionID(),
		); err != nil {
			return err
		}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("sqlite: commit waiting tree cancel: %w", err)
	}
	return nil
}
