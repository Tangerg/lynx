package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	rundomain "github.com/Tangerg/lynx/app2/runtime/domain/run"
	"github.com/Tangerg/lynx/app2/runtime/runflow"
)

func (database *Database) CommitRunItemEvents(ctx context.Context, write runflow.RunItemEventWrite) error {
	return database.commitLiveRun(ctx, "item", write.Run, write.ExpectedSegmentID, func(transaction *sql.Tx) error {
		if err := putItem(ctx, transaction, write.Item); err != nil {
			return err
		}
		if write.ToolResult != nil {
			if err := insertToolResult(ctx, transaction, *write.ToolResult); err != nil {
				return err
			}
		}
		return insertRunEvents(ctx, transaction, write.Events)
	})
}

func (database *Database) CommitRunEvent(ctx context.Context, write runflow.RunEventWrite) error {
	return database.commitLiveRun(ctx, "event", write.Run, write.ExpectedSegmentID, func(transaction *sql.Tx) error {
		return insertRunEvents(ctx, transaction, []rundomain.EventRecord{write.Event})
	})
}

func (database *Database) commitLiveRun(
	ctx context.Context,
	material string,
	record rundomain.Record,
	expectedSegmentID string,
	commitMaterial func(*sql.Tx) error,
) error {
	transaction, err := database.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: begin live Run %s: %w", material, err)
	}
	defer transaction.Rollback()
	result, err := transaction.ExecContext(ctx, `
		UPDATE runs SET body=?, updated_at=?
		WHERE id=? AND status='running' AND active_segment_id=?`,
		string(record.Body), encodeTime(record.Run.UpdatedAt()),
		record.Run.ID(), expectedSegmentID,
	)
	if err != nil {
		return fmt.Errorf("sqlite: advance live Run %s: %w", material, err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect live Run %s: %w", material, err)
	}
	if changed != 1 {
		return rundomain.ErrInvalidTransition
	}
	if err := commitMaterial(transaction); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("sqlite: commit live Run %s: %w", material, err)
	}
	return nil
}
