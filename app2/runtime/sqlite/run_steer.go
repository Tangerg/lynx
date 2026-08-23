package sqlite

import (
	"context"
	"fmt"

	rundomain "github.com/Tangerg/lynx/app2/runtime/domain/run"
	"github.com/Tangerg/lynx/app2/runtime/runflow"
)

func (database *Database) CommitSteer(ctx context.Context, write runflow.SteerWrite) error {
	transaction, err := database.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	result, err := transaction.ExecContext(ctx, `UPDATE runs SET body=?,updated_at=? WHERE id=? AND status='running' AND active_segment_id=?`, string(write.Run.Body), encodeTime(write.Run.Run.UpdatedAt()), write.Run.Run.ID(), write.ExpectedSegmentID)
	if err != nil {
		return err
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return rundomain.ErrInvalidTransition
	}
	if err := insertItem(ctx, transaction, write.Item); err != nil {
		return err
	}
	if err := insertConversationMessage(ctx, transaction, write.Message); err != nil {
		return err
	}
	if err := insertRunEvents(ctx, transaction, []rundomain.EventRecord{write.Event}); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("sqlite: commit steer: %w", err)
	}
	return nil
}
