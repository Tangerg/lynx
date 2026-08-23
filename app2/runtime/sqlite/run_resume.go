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

func (database *Database) CommitResume(ctx context.Context, write runflow.ResumeWrite) error {
	transaction, err := database.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: begin run resume: %w", err)
	}
	defer transaction.Rollback()

	var pendingBody string
	err = transaction.QueryRowContext(ctx, `SELECT body FROM interrupt_sets WHERE run_id = ?`, write.Run.Run.ID()).Scan(&pendingBody)
	if errors.Is(err, sql.ErrNoRows) {
		return runflow.ErrInterruptSetNotFound
	}
	if err != nil {
		return fmt.Errorf("sqlite: read resume interrupt: %w", err)
	}
	var current any
	var expected any
	if err := json.Unmarshal([]byte(pendingBody), &current); err != nil {
		return fmt.Errorf("sqlite: decode resume interrupt: %w", err)
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

	value := write.Run.Run
	result, err := transaction.ExecContext(ctx, `
		UPDATE runs SET status = ?, active_segment_id = ?, body = ?, updated_at = ?
		WHERE id = ? AND status = 'waiting' AND active_segment_id IS NULL`,
		string(value.Status()), value.ActiveSegmentID(), string(write.Run.Body), encodeTime(value.UpdatedAt()), value.ID())
	if err != nil {
		return fmt.Errorf("sqlite: open resumed segment: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return rundomain.ErrInvalidTransition
	}
	for _, item := range write.UpdatedItems {
		result, err := transaction.ExecContext(ctx, `
			UPDATE items SET body = ?, search_text = ?
			WHERE id = ? AND session_id = ? AND run_id = ?`,
			string(item.Body), string(item.SearchText), item.ID, item.SessionID, item.RunID,
		)
		if err != nil {
			return fmt.Errorf("sqlite: update answered item %s: %w", item.ID, err)
		}
		changed, _ := result.RowsAffected()
		if changed != 1 {
			return fmt.Errorf("sqlite: answered item %s is missing", item.ID)
		}
	}
	if write.OpeningItem != nil {
		if err := insertItem(ctx, transaction, *write.OpeningItem); err != nil {
			return err
		}
	}
	if write.OpeningMessage != nil {
		if err := insertConversationMessage(ctx, transaction, *write.OpeningMessage); err != nil { return err }
	}
	if err := insertRunEvents(ctx, transaction, write.Events); err != nil {
		return err
	}
	if _, err := transaction.ExecContext(ctx, `DELETE FROM interrupt_sets WHERE run_id = ?`, value.ID()); err != nil {
		return fmt.Errorf("sqlite: consume interrupt set: %w", err)
	}
	if _, err := transaction.ExecContext(ctx, `DELETE FROM executor_checkpoints WHERE run_id = ?`, value.ID()); err != nil {
		return fmt.Errorf("sqlite: consume executor checkpoint: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("sqlite: commit run resume: %w", err)
	}
	return nil
}
