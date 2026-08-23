package sqlite

import (
	"context"
	"fmt"

	rundomain "github.com/Tangerg/lynx/app2/runtime/domain/run"
	"github.com/Tangerg/lynx/app2/runtime/runflow"
)

func (database *Database) CommitRunEvent(ctx context.Context, write runflow.RunEventWrite) error {
	transaction, err := database.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: begin live Run event: %w", err)
	}
	defer transaction.Rollback()
	result, err := transaction.ExecContext(ctx, `
		UPDATE runs SET body=?, updated_at=?
		WHERE id=? AND status='running' AND active_segment_id=?`,
		string(write.Run.Body), encodeTime(write.Run.Run.UpdatedAt()),
		write.Run.Run.ID(), write.ExpectedSegmentID,
	)
	if err != nil {
		return fmt.Errorf("sqlite: advance live Run event: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect live Run event: %w", err)
	}
	if changed != 1 {
		return rundomain.ErrInvalidTransition
	}
	if err := insertRunEvents(ctx, transaction, []rundomain.EventRecord{write.Event}); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("sqlite: commit live Run event: %w", err)
	}
	return nil
}
