package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	rundomain "github.com/Tangerg/lynx/app2/runtime/domain/run"
	"github.com/Tangerg/lynx/app2/runtime/runflow"
)

func (database *Database) CommitWait(ctx context.Context, write runflow.WaitWrite) error {
	transaction, err := database.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: begin run wait: %w", err)
	}
	defer transaction.Rollback()
	value := write.Run.Run
	result, err := transaction.ExecContext(ctx, `
		UPDATE runs SET status = ?, active_segment_id = NULL, body = ?, updated_at = ?
		WHERE id = ? AND status = 'running' AND active_segment_id = ?`,
		string(value.Status()), string(write.Run.Body), encodeTime(value.UpdatedAt()), value.ID(), write.ExpectedSegmentID)
	if err != nil {
		return fmt.Errorf("sqlite: park run: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return rundomain.ErrInvalidTransition
	}
	for _, item := range write.Items {
		if err := putItem(ctx, transaction, item); err != nil { return err }
	}
	for _, message := range write.Messages {
		if err := insertConversationMessage(ctx, transaction, message); err != nil { return err }
	}
	for _, result := range write.ToolResults {
		if err := insertToolResult(ctx, transaction, result); err != nil { return err }
	}
	if err := insertRunEvents(ctx, transaction, write.Events); err != nil {
		return err
	}
	interruptBody, err := encodeJSON(write.Interrupts)
	if err != nil {
		return err
	}
	now := encodeTime(value.UpdatedAt())
	if _, err := transaction.ExecContext(ctx, `INSERT INTO interrupt_sets(run_id,session_id,body,created_at,updated_at) VALUES(?,?,?,?,?)`, value.ID(), value.SessionID(), interruptBody, now, now); err != nil {
		return fmt.Errorf("sqlite: persist interrupt set: %w", err)
	}
	if len(write.Checkpoint) == 0 {
		return errors.New("sqlite: waiting run checkpoint is empty")
	}
	if _, err := transaction.ExecContext(ctx, `INSERT INTO executor_checkpoints(run_id,body,updated_at) VALUES(?,?,?)`, value.ID(), write.Checkpoint, now); err != nil {
		return fmt.Errorf("sqlite: persist executor checkpoint: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("sqlite: commit run wait: %w", err)
	}
	return nil
}

func (database *Database) GetExecutorCheckpoint(ctx context.Context, runID string) ([]byte, error) {
	var body []byte
	err := database.database.QueryRowContext(ctx, `SELECT body FROM executor_checkpoints WHERE run_id = ?`, runID).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, runflow.ErrInterruptSetNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite: get executor checkpoint: %w", err)
	}
	return body, nil
}

func encodeJSON(value any) (string, error) {
	body, err := json.Marshal(value)
	return string(body), err
}
