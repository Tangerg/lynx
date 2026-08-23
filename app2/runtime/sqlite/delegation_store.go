package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app2/runtime/domain/delegation"
)

// ReserveDelegateAdmission installs one private executor-to-product binding.
// Replays return the previously allocated product identities; a caller can
// never replace them by racing a second reservation.
func (database *Database) ReserveDelegateAdmission(
	ctx context.Context,
	requested delegation.Admission,
) (delegation.Admission, error) {
	if err := requested.Validate(); err != nil || requested.Status != delegation.Pending {
		return delegation.Admission{}, errors.Join(delegation.ErrInvalidAdmission, err)
	}
	transaction, err := database.database.BeginTx(ctx, nil)
	if err != nil {
		return delegation.Admission{}, fmt.Errorf("sqlite: begin delegate admission: %w", err)
	}
	defer transaction.Rollback()
	_, err = transaction.ExecContext(ctx, `
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
		return delegation.Admission{}, fmt.Errorf("sqlite: reserve delegate admission %s: %w", requested.MemberID, err)
	}
	existing, err := scanDelegateAdmission(transaction.QueryRowContext(ctx, delegateAdmissionSelect+` WHERE member_id = ?`, requested.MemberID))
	if err != nil {
		return delegation.Admission{}, err
	}
	if !existing.SameReservation(requested) {
		return delegation.Admission{}, delegation.ErrAdmissionConflict
	}
	if err := transaction.Commit(); err != nil {
		return delegation.Admission{}, fmt.Errorf("sqlite: commit delegate admission: %w", err)
	}
	return existing, nil
}

func (database *Database) GetDelegateAdmission(
	ctx context.Context,
	memberID string,
) (delegation.Admission, error) {
	return scanDelegateAdmission(database.database.QueryRowContext(ctx, delegateAdmissionSelect+` WHERE member_id = ?`, memberID))
}

func (database *Database) UpdateDelegateAdmission(
	ctx context.Context,
	value delegation.Admission,
	expected delegation.Status,
) error {
	if err := value.Validate(); err != nil {
		return err
	}
	result, err := database.database.ExecContext(ctx, `
		UPDATE delegate_admissions SET status = ?, failure = ?, updated_at = ?
		WHERE member_id = ? AND status = ?`,
		string(value.Status), value.Failure, encodeTime(value.UpdatedAt), value.MemberID, string(expected),
	)
	if err != nil {
		return fmt.Errorf("sqlite: conclude delegate admission %s: %w", value.MemberID, err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect delegate admission conclusion: %w", err)
	}
	if changed == 0 {
		current, lookupErr := database.GetDelegateAdmission(ctx, value.MemberID)
		if lookupErr == nil && current.Status == value.Status && current.Failure == value.Failure {
			return nil
		}
		return delegation.ErrAdmissionConflict
	}
	return nil
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
