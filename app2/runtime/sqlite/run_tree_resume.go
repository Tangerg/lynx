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

// CommitTreeResume atomically consumes one root interrupt set and opens a new
// Segment generation for every parked member. Events retain postorder so no
// child can appear to resume behind an already-running ancestor.
func (database *Database) CommitTreeResume(ctx context.Context, write runflow.TreeResumeWrite) error {
	if len(write.Runs) == 0 || write.ExpectedInterrupts.RootRunID == "" ||
		len(write.ExpectedInterrupts.Interrupts) == 0 {
		return errors.New("sqlite: tree resume is incomplete")
	}
	root := write.Runs[len(write.Runs)-1]
	if root.Run.Run.ID() != write.ExpectedInterrupts.RootRunID || root.Depth != 0 ||
		root.Run.Run.ParentRunID() != "" || root.Run.Run.Status() != rundomain.Running {
		return errors.New("sqlite: tree resume root is invalid")
	}
	transaction, err := database.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: begin tree resume: %w", err)
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
		return fmt.Errorf("sqlite: read tree resume interrupt: %w", err)
	}
	var current any
	var expected any
	if err := json.Unmarshal([]byte(pendingBody), &current); err != nil {
		return fmt.Errorf("sqlite: decode tree resume interrupt: %w", err)
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
		return fmt.Errorf("sqlite: count resumed tree members: %w", err)
	}
	if openMembers != len(write.Runs) {
		return errors.New("sqlite: tree resume does not cover every open Run")
	}
	lastDepth := ^uint32(0)
	knownRuns := make(map[string]bool, len(write.Runs))
	for _, member := range write.Runs {
		value := member.Run.Run
		if member.Depth > lastDepth || value.Status() != rundomain.Running ||
			value.ActiveSegmentID() == "" || value.SessionID() != write.ExpectedInterrupts.SessionID ||
			knownRuns[value.ID()] {
			return errors.New("sqlite: resumed tree members are invalid or out of order")
		}
		lastDepth = member.Depth
		knownRuns[value.ID()] = true
		result, err := transaction.ExecContext(ctx, `
			UPDATE runs SET status = ?, active_segment_id = ?, body = ?, updated_at = ?
			WHERE id = ? AND status = 'waiting' AND active_segment_id IS NULL`,
			string(value.Status()), value.ActiveSegmentID(), string(member.Run.Body),
			encodeTime(value.UpdatedAt()), value.ID(),
		)
		if err != nil {
			return fmt.Errorf("sqlite: resume tree Run %s: %w", value.ID(), err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return rundomain.ErrInvalidTransition
		}
		if err := insertRunEvents(ctx, transaction, member.Events); err != nil {
			return err
		}
	}
	for _, item := range write.UpdatedItems {
		if !knownRuns[item.RunID] || item.SessionID != write.ExpectedInterrupts.SessionID {
			return errors.New("sqlite: answered tree Item changed ownership")
		}
		result, err := transaction.ExecContext(ctx, `
			UPDATE items SET body = ? WHERE id = ? AND session_id = ? AND run_id = ?`,
			string(item.Body), item.ID, item.SessionID, item.RunID,
		)
		if err != nil {
			return fmt.Errorf("sqlite: update answered tree Item %s: %w", item.ID, err)
		}
		changed, _ := result.RowsAffected()
		if changed != 1 {
			return fmt.Errorf("sqlite: answered tree Item %s is missing", item.ID)
		}
	}
	if write.OpeningItem != nil {
		if write.OpeningItem.RunID != root.Run.Run.ID() {
			return errors.New("sqlite: tree resume opening Item is not root-owned")
		}
		if err := insertItem(ctx, transaction, *write.OpeningItem); err != nil {
			return err
		}
	}
	if write.OpeningMessage != nil {
		if write.OpeningMessage.RunID != root.Run.Run.ID() {
			return errors.New("sqlite: tree resume opening Conversation message is not root-owned")
		}
		if err := insertConversationMessage(ctx, transaction, *write.OpeningMessage); err != nil {
			return err
		}
	}
	if _, err := transaction.ExecContext(ctx, `DELETE FROM interrupt_sets WHERE run_id = ?`,
		write.ExpectedInterrupts.RootRunID,
	); err != nil {
		return fmt.Errorf("sqlite: consume tree interrupt set: %w", err)
	}
	if _, err := transaction.ExecContext(ctx, `DELETE FROM executor_checkpoints WHERE run_id = ?`,
		write.ExpectedInterrupts.RootRunID,
	); err != nil {
		return fmt.Errorf("sqlite: consume tree checkpoint: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("sqlite: commit tree resume: %w", err)
	}
	return nil
}
