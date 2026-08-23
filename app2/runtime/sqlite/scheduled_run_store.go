package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app2/runtime/domain/schedule"
	"github.com/Tangerg/lynx/app2/runtime/runflow"
)

// CreateScheduledRun is the single commit point for a scheduled Session, its
// opening Run material, occurrence acceptance, and Schedule run timestamp.
// Returning created=false means another Runtime already accepted this exact
// durable occurrence; the caller must not publish or launch it again.
func (database *Database) CreateScheduledRun(
	ctx context.Context,
	write runflow.ScheduledRunWrite,
) (bool, error) {
	if write.ScheduleID == "" || write.Session.ID().String() == "" ||
		write.Run.Run.ID() == "" || write.FiredAt.IsZero() || write.AcceptedAt.IsZero() {
		return false, errors.New("sqlite: scheduled Run write is incomplete")
	}
	if write.Session.ID().String() != write.Run.Run.SessionID() ||
		write.Opening.SessionID != write.Run.Run.SessionID() ||
		write.Opening.RunID != write.Run.Run.ID() ||
		write.OpeningMessage.SessionID != write.Run.Run.SessionID() ||
		write.OpeningMessage.RunID != write.Run.Run.ID() {
		return false, errors.New("sqlite: scheduled Run material has inconsistent ownership")
	}

	transaction, err := database.database.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("sqlite: begin scheduled Run admission: %w", err)
	}
	defer transaction.Rollback()

	if write.OccurrenceID == "" {
		if write.AllowMissingSchedule {
			return false, errors.New("sqlite: manual scheduled Run cannot allow a missing Schedule")
		}
		if _, err := getScheduleTx(ctx, transaction, write.ScheduleID); err != nil {
			return false, err
		}
	} else {
		if !write.AllowMissingSchedule {
			return false, errors.New("sqlite: durable occurrence must survive Schedule deletion")
		}
		accepted, err := inspectOccurrenceAdmission(ctx, transaction, write)
		if err != nil {
			return false, err
		}
		if accepted {
			return false, nil
		}
	}

	if err := insertSessionTx(ctx, transaction, write.Session); err != nil {
		return false, fmt.Errorf("sqlite: create scheduled Session %s: %w", write.Session.ID(), err)
	}
	if err := insertRun(ctx, transaction, write.Run); err != nil {
		return false, err
	}
	if err := insertItem(ctx, transaction, write.Opening); err != nil {
		return false, err
	}
	if err := insertConversationMessage(ctx, transaction, write.OpeningMessage); err != nil {
		return false, err
	}
	if err := insertRunEvents(ctx, transaction, write.Events); err != nil {
		return false, err
	}
	if err := recordScheduleRunTx(ctx, transaction, write); err != nil {
		return false, err
	}
	if write.OccurrenceID != "" {
		result, err := transaction.ExecContext(ctx, `
			UPDATE schedule_occurrences SET status = 'accepted', accepted_at = ?
			WHERE id = ? AND schedule_id = ? AND session_id = ? AND run_id = ?
				AND status = 'pending'`,
			encodeTime(write.AcceptedAt), write.OccurrenceID, write.ScheduleID,
			write.Session.ID().String(), write.Run.Run.ID(),
		)
		if err != nil {
			return false, fmt.Errorf("sqlite: accept schedule occurrence %q: %w", write.OccurrenceID, err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return false, fmt.Errorf("sqlite: inspect schedule occurrence acceptance %q: %w", write.OccurrenceID, err)
		}
		if changed != 1 {
			return false, fmt.Errorf("sqlite: schedule occurrence %q changed during admission", write.OccurrenceID)
		}
	}
	if err := transaction.Commit(); err != nil {
		return false, fmt.Errorf("sqlite: commit scheduled Run admission: %w", err)
	}
	return true, nil
}

func inspectOccurrenceAdmission(
	ctx context.Context,
	transaction *sql.Tx,
	write runflow.ScheduledRunWrite,
) (bool, error) {
	var scheduleID, sessionID, runID, status string
	err := transaction.QueryRowContext(ctx, `
		SELECT schedule_id, session_id, run_id, status
		FROM schedule_occurrences WHERE id = ?`, write.OccurrenceID,
	).Scan(&scheduleID, &sessionID, &runID, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("sqlite: schedule occurrence %q does not exist", write.OccurrenceID)
	}
	if err != nil {
		return false, fmt.Errorf("sqlite: inspect schedule occurrence %q: %w", write.OccurrenceID, err)
	}
	if scheduleID != write.ScheduleID || sessionID != write.Session.ID().String() ||
		runID != write.Run.Run.ID() {
		return false, fmt.Errorf("sqlite: schedule occurrence %q identity conflicts with admission", write.OccurrenceID)
	}
	switch schedule.OccurrenceStatus(status) {
	case schedule.OccurrencePending:
		return false, nil
	case schedule.OccurrenceAccepted:
		var existingSession string
		err := transaction.QueryRowContext(ctx, `SELECT session_id FROM runs WHERE id = ?`, runID).Scan(&existingSession)
		if err != nil {
			return false, fmt.Errorf("sqlite: accepted schedule occurrence %q has no Run: %w", write.OccurrenceID, err)
		}
		if existingSession != sessionID {
			return false, fmt.Errorf("sqlite: accepted schedule occurrence %q Run owner changed", write.OccurrenceID)
		}
		return true, nil
	default:
		return false, fmt.Errorf("sqlite: schedule occurrence %q has invalid status %q", write.OccurrenceID, status)
	}
}

func recordScheduleRunTx(
	ctx context.Context,
	transaction *sql.Tx,
	write runflow.ScheduledRunWrite,
) error {
	value, err := getScheduleTx(ctx, transaction, write.ScheduleID)
	if errors.Is(err, schedule.ErrNotFound) && write.AllowMissingSchedule {
		return nil
	}
	if err != nil {
		return err
	}
	updated, changed, err := value.RecordRun(write.FiredAt, write.AcceptedAt)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	result, err := transaction.ExecContext(ctx, `
		UPDATE schedules SET last_run_at = ?, revision = ?, updated_at = ?
		WHERE id = ? AND revision = ?`,
		encodeTime(updated.LastRunAt()), updated.Revision(), encodeTime(updated.UpdatedAt()),
		updated.ID(), value.Revision(),
	)
	if err != nil {
		return fmt.Errorf("sqlite: record schedule %q Run: %w", updated.ID(), err)
	}
	changedRows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect schedule %q Run record: %w", updated.ID(), err)
	}
	if changedRows != 1 {
		return schedule.ErrRevisionConflict
	}
	return nil
}

func getScheduleTx(ctx context.Context, transaction *sql.Tx, id string) (schedule.Schedule, error) {
	value, err := scanSchedule(transaction.QueryRowContext(ctx, scheduleSelect+` WHERE id = ?`, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return schedule.Schedule{}, schedule.ErrNotFound
	}
	if err != nil {
		return schedule.Schedule{}, fmt.Errorf("sqlite: get schedule %q in transaction: %w", id, err)
	}
	return value, nil
}
