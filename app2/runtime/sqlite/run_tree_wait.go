package sqlite

import (
	"context"
	"errors"
	"fmt"

	rundomain "github.com/Tangerg/lynx/app2/runtime/domain/run"
	"github.com/Tangerg/lynx/app2/runtime/runflow"
)

// CommitTreeWait parks every open member of one Run tree, appends their
// source-owned material in child-before-parent order, and installs the one
// root-owned continuation fact atomically.
func (database *Database) CommitTreeWait(ctx context.Context, write runflow.TreeWaitWrite) error {
	if len(write.Runs) == 0 || len(write.Checkpoint) == 0 ||
		write.Interrupts.RootRunID == "" || write.Interrupts.SessionID == "" ||
		len(write.Interrupts.Interrupts) == 0 {
		return errors.New("sqlite: tree wait is incomplete")
	}
	root := write.Runs[len(write.Runs)-1]
	if root.Run.Run.ID() != write.Interrupts.RootRunID || root.Run.Run.ParentRunID() != "" ||
		root.Run.Run.Status() != rundomain.Waiting || root.Depth != 0 {
		return errors.New("sqlite: tree wait root is invalid")
	}
	transaction, err := database.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: begin tree wait: %w", err)
	}
	defer transaction.Rollback()
	var openMembers int
	if err := transaction.QueryRowContext(ctx, `
		SELECT count(*) FROM runs
		WHERE status != 'finished' AND (id = ? OR root_run_id = ?)`,
		write.Interrupts.RootRunID, write.Interrupts.RootRunID,
	).Scan(&openMembers); err != nil {
		return fmt.Errorf("sqlite: count open tree members: %w", err)
	}
	if openMembers != len(write.Runs) {
		return errors.New("sqlite: tree wait does not cover every open Run")
	}
	lastDepth := ^uint32(0)
	for _, member := range write.Runs {
		value := member.Run.Run
		if member.Depth > lastDepth || value.Status() != rundomain.Waiting ||
			value.SessionID() != write.Interrupts.SessionID {
			return errors.New("sqlite: tree wait members are invalid or out of order")
		}
		lastDepth = member.Depth
		result, err := transaction.ExecContext(ctx, `
			UPDATE runs SET status = ?, active_segment_id = NULL, body = ?, updated_at = ?
			WHERE id = ? AND status = 'running' AND active_segment_id = ?`,
			string(value.Status()), string(member.Run.Body), encodeTime(value.UpdatedAt()),
			value.ID(), member.ExpectedSegmentID,
		)
		if err != nil {
			return fmt.Errorf("sqlite: park tree Run %s: %w", value.ID(), err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return rundomain.ErrInvalidTransition
		}
		for _, item := range member.Items {
			if err := putItem(ctx, transaction, item); err != nil {
				return err
			}
		}
		for _, message := range member.Messages {
			if err := insertConversationMessage(ctx, transaction, message); err != nil {
				return err
			}
		}
		for _, result := range member.ToolResults {
			if err := insertToolResult(ctx, transaction, result); err != nil {
				return err
			}
		}
		if err := insertRunEvents(ctx, transaction, member.Events); err != nil {
			return err
		}
	}
	interruptBody, err := encodeJSON(write.Interrupts)
	if err != nil {
		return err
	}
	now := encodeTime(root.Run.Run.UpdatedAt())
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO interrupt_sets(run_id,session_id,body,created_at,updated_at)
		VALUES(?,?,?,?,?)`,
		write.Interrupts.RootRunID, write.Interrupts.SessionID, interruptBody, now, now,
	); err != nil {
		return fmt.Errorf("sqlite: persist tree interrupt set: %w", err)
	}
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO executor_checkpoints(run_id,body,updated_at) VALUES(?,?,?)`,
		write.Interrupts.RootRunID, write.Checkpoint, now,
	); err != nil {
		return fmt.Errorf("sqlite: persist tree checkpoint: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("sqlite: commit tree wait: %w", err)
	}
	return nil
}
