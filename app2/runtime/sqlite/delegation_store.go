package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app2/runtime/domain/delegation"
	rundomain "github.com/Tangerg/lynx/app2/runtime/domain/run"
	"github.com/Tangerg/lynx/app2/runtime/runflow"
)

// CommitDelegateAdmission installs one private executor-to-product binding and
// the parent ToolCall that spawned it. Replays return the previously allocated
// identities without appending a second Item or event.
func (database *Database) CommitDelegateAdmission(
	ctx context.Context,
	write runflow.DelegateAdmissionWrite,
) (delegation.Admission, bool, error) {
	requested := write.Admission
	if err := requested.Validate(); err != nil || requested.Status != delegation.Pending {
		return delegation.Admission{}, false, errors.Join(delegation.ErrInvalidAdmission, err)
	}
	transaction, err := database.database.BeginTx(ctx, nil)
	if err != nil {
		return delegation.Admission{}, false, fmt.Errorf("sqlite: begin delegate admission: %w", err)
	}
	defer transaction.Rollback()
	result, err := transaction.ExecContext(ctx, `
		INSERT INTO delegate_admissions (
			member_id, parent_member_id, child_key, run_id, segment_id,
			session_id, parent_run_id, root_run_id, spawned_by_item_id,
			provider, model, summary, instructions, status, failure, started_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(member_id) DO NOTHING`,
		requested.MemberID, requested.ParentMemberID, requested.ChildKey,
		requested.RunID, requested.SegmentID, requested.SessionID,
		requested.ParentRunID, requested.RootRunID, requested.SpawnedByItemID,
		requested.Provider, requested.Model, requested.Summary, requested.Instructions,
		string(requested.Status), requested.Failure,
		encodeTime(requested.StartedAt), encodeTime(requested.UpdatedAt),
	)
	if err != nil {
		return delegation.Admission{}, false, fmt.Errorf("sqlite: reserve delegate admission %s: %w", requested.MemberID, err)
	}
	existing, err := scanDelegateAdmission(transaction.QueryRowContext(ctx, delegateAdmissionSelect+` WHERE member_id = ?`, requested.MemberID))
	if err != nil {
		return delegation.Admission{}, false, err
	}
	if !existing.SameReservation(requested) {
		return delegation.Admission{}, false, delegation.ErrAdmissionConflict
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return delegation.Admission{}, false, fmt.Errorf("sqlite: inspect delegate admission insert: %w", err)
	}
	if inserted > 0 {
		if err := updateActiveRun(ctx, transaction, write.Parent, write.ExpectedSegmentID); err != nil {
			return delegation.Admission{}, false, err
		}
		if err := putItem(ctx, transaction, write.Item); err != nil {
			return delegation.Admission{}, false, err
		}
		if err := insertRunEvents(ctx, transaction, []rundomain.EventRecord{write.Event}); err != nil {
			return delegation.Admission{}, false, err
		}
	}
	if err := transaction.Commit(); err != nil {
		return delegation.Admission{}, false, fmt.Errorf("sqlite: commit delegate admission: %w", err)
	}
	return existing, inserted > 0, nil
}

func (database *Database) GetDelegateAdmission(
	ctx context.Context,
	memberID string,
) (delegation.Admission, error) {
	return scanDelegateAdmission(database.database.QueryRowContext(ctx, delegateAdmissionSelect+` WHERE member_id = ?`, memberID))
}

func (database *Database) CommitDelegateStart(
	ctx context.Context,
	write runflow.DelegateStartWrite,
) (bool, error) {
	value := write.Admission
	if err := value.Validate(); err != nil {
		return false, err
	}
	if value.Status != delegation.Started || write.Child.Run.ID() != value.RunID ||
		write.Child.Run.ActiveSegmentID() != value.SegmentID {
		return false, delegation.ErrAdmissionConflict
	}
	transaction, err := database.database.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("sqlite: begin delegate start: %w", err)
	}
	defer transaction.Rollback()
	current, err := scanDelegateAdmission(transaction.QueryRowContext(ctx, delegateAdmissionSelect+` WHERE member_id = ?`, value.MemberID))
	if err != nil {
		return false, err
	}
	if !current.SameReservation(value) {
		return false, delegation.ErrAdmissionConflict
	}
	if current.Status == delegation.Started {
		stored, lookupErr := scanRun(transaction.QueryRowContext(ctx, runByIDQuery, value.RunID))
		if lookupErr != nil || !sameRunIdentity(stored.Run, write.Child.Run) {
			return false, delegation.ErrAdmissionConflict
		}
		return false, transaction.Commit()
	}
	if current.Status != delegation.Pending {
		return false, delegation.ErrAdmissionConflict
	}
	result, err := transaction.ExecContext(ctx, `
		UPDATE delegate_admissions SET status = ?, failure = ?, updated_at = ?
		WHERE member_id = ? AND status = ?`,
		string(value.Status), value.Failure, encodeTime(value.UpdatedAt), value.MemberID, string(delegation.Pending),
	)
	if err != nil {
		return false, fmt.Errorf("sqlite: conclude delegate admission %s: %w", value.MemberID, err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("sqlite: inspect delegate admission conclusion: %w", err)
	}
	if changed != 1 {
		return false, delegation.ErrAdmissionConflict
	}
	if err := insertRun(ctx, transaction, write.Child); err != nil {
		return false, err
	}
	if err := insertRunEvents(ctx, transaction, []rundomain.EventRecord{write.Event}); err != nil {
		return false, err
	}
	if err := transaction.Commit(); err != nil {
		return false, fmt.Errorf("sqlite: commit delegate start: %w", err)
	}
	return true, nil
}

func (database *Database) CommitDelegateAbort(
	ctx context.Context,
	write runflow.DelegateAbortWrite,
) (bool, error) {
	value := write.Admission
	if err := value.Validate(); err != nil {
		return false, err
	}
	if value.Status != delegation.Aborted {
		return false, delegation.ErrAdmissionConflict
	}
	transaction, err := database.database.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("sqlite: begin delegate abort: %w", err)
	}
	defer transaction.Rollback()
	current, err := scanDelegateAdmission(transaction.QueryRowContext(ctx, delegateAdmissionSelect+` WHERE member_id = ?`, value.MemberID))
	if err != nil {
		return false, err
	}
	if !current.SameReservation(value) {
		return false, delegation.ErrAdmissionConflict
	}
	if current.Status == delegation.Aborted {
		if current.Failure != value.Failure {
			return false, delegation.ErrAdmissionConflict
		}
		return false, transaction.Commit()
	}
	if current.Status != delegation.Pending {
		return false, delegation.ErrAdmissionConflict
	}
	result, err := transaction.ExecContext(ctx, `
		UPDATE delegate_admissions SET status = ?, failure = ?, updated_at = ?
		WHERE member_id = ? AND status = ?`,
		string(value.Status), value.Failure, encodeTime(value.UpdatedAt), value.MemberID, string(delegation.Pending),
	)
	if err != nil {
		return false, fmt.Errorf("sqlite: abort delegate admission %s: %w", value.MemberID, err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("sqlite: inspect delegate abort: %w", err)
	}
	if changed != 1 {
		return false, delegation.ErrAdmissionConflict
	}
	if err := updateActiveRun(ctx, transaction, write.Parent, write.ExpectedSegmentID); err != nil {
		return false, err
	}
	if err := putItem(ctx, transaction, write.Item); err != nil {
		return false, err
	}
	if err := insertRunEvents(ctx, transaction, []rundomain.EventRecord{write.Event}); err != nil {
		return false, err
	}
	if err := transaction.Commit(); err != nil {
		return false, fmt.Errorf("sqlite: commit delegate abort: %w", err)
	}
	return true, nil
}

const runByIDQuery = `
	SELECT id, session_id, coalesce(parent_run_id, ''), coalesce(root_run_id, ''),
		coalesce(spawned_by_item_id, ''), status, coalesce(active_segment_id, ''),
		provider, model, coalesce(outcome, ''), detail, body, created_at, updated_at, coalesce(finished_at, '')
	FROM runs WHERE id = ?`

func updateActiveRun(ctx context.Context, transaction *sql.Tx, record rundomain.Record, expectedSegmentID string) error {
	value := record.Run
	result, err := transaction.ExecContext(ctx, `
		UPDATE runs SET body = ?, updated_at = ?
		WHERE id = ? AND status = 'running' AND active_segment_id = ?`,
		string(record.Body), encodeTime(value.UpdatedAt()), value.ID(), expectedSegmentID,
	)
	if err != nil {
		return fmt.Errorf("sqlite: advance delegate parent %s: %w", value.ID(), err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect delegate parent advance: %w", err)
	}
	if changed != 1 {
		return rundomain.ErrInvalidTransition
	}
	return nil
}

func sameRunIdentity(left, right rundomain.Run) bool {
	return left.ID() == right.ID() && left.SessionID() == right.SessionID() &&
		left.ParentRunID() == right.ParentRunID() && left.RootRunID() == right.RootRunID() &&
		left.SpawnedByItemID() == right.SpawnedByItemID() &&
		left.Provider() == right.Provider() && left.Model() == right.Model() &&
		left.CreatedAt().Equal(right.CreatedAt())
}

const delegateAdmissionSelect = `
	SELECT member_id, parent_member_id, child_key, run_id, segment_id,
		session_id, parent_run_id, root_run_id, spawned_by_item_id,
		provider, model, summary, instructions, status, failure, started_at, updated_at
	FROM delegate_admissions`

func scanDelegateAdmission(row rowScanner) (delegation.Admission, error) {
	var value delegation.Admission
	var status, startedAt, updatedAt string
	err := row.Scan(
		&value.MemberID, &value.ParentMemberID, &value.ChildKey,
		&value.RunID, &value.SegmentID, &value.SessionID,
		&value.ParentRunID, &value.RootRunID, &value.SpawnedByItemID,
		&value.Provider, &value.Model, &value.Summary, &value.Instructions,
		&status, &value.Failure, &startedAt, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return delegation.Admission{}, delegation.ErrNotFound
	}
	if err != nil {
		return delegation.Admission{}, fmt.Errorf("sqlite: scan delegate admission: %w", err)
	}
	value.Status = delegation.Status(status)
	value.StartedAt, err = decodeTime(startedAt)
	if err != nil {
		return delegation.Admission{}, err
	}
	value.UpdatedAt, err = decodeTime(updatedAt)
	if err != nil {
		return delegation.Admission{}, err
	}
	return delegation.Rehydrate(value)
}
