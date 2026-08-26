package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalidChildRunStartReservation  = errors.New("sqlite: invalid child Run start reservation")
	ErrChildRunStartReservationConflict = errors.New("sqlite: child Run start reservation conflict")
)

// ChildRunStartConclusion is the durable conclusion of an opaque reservation.
// Storage does not interpret the payload or publish product state.
type ChildRunStartConclusion string

const (
	ChildRunStartConclusionStarted ChildRunStartConclusion = "started"
	ChildRunStartConclusionAborted ChildRunStartConclusion = "aborted"
)

func (c ChildRunStartConclusion) valid() bool {
	return c == ChildRunStartConclusionStarted || c == ChildRunStartConclusionAborted
}

const childRunStartReserved = "reserved"

// ChildRunStartReservationRecord is SQLite's opaque technical record. Runtime
// application semantics live in the runsegment adapter; storage only enforces
// exact identity and compare-before-delete behavior.
type ChildRunStartReservationRecord struct {
	MemberID  string
	SessionID string
	Payload   []byte
	CreatedAt time.Time
}

func (c ChildRunStartReservationRecord) validate() error {
	if strings.TrimSpace(c.MemberID) == "" || c.MemberID != strings.TrimSpace(c.MemberID) {
		return fmt.Errorf("%w: member ID", ErrInvalidChildRunStartReservation)
	}
	if strings.TrimSpace(c.SessionID) == "" || c.SessionID != strings.TrimSpace(c.SessionID) {
		return fmt.Errorf("%w: session ID", ErrInvalidChildRunStartReservation)
	}
	if len(c.Payload) == 0 {
		return fmt.Errorf("%w: payload", ErrInvalidChildRunStartReservation)
	}
	if c.CreatedAt.IsZero() {
		return fmt.Errorf("%w: creation time", ErrInvalidChildRunStartReservation)
	}
	return nil
}

// ChildRunStartReservationStore persists opaque child-start reservations and
// their conclusive state. Rows intentionally survive conclusion so exact
// callbacks remain idempotent without guessing from absence.
type ChildRunStartReservationStore struct{ db *sql.DB }

func NewChildRunStartReservationStore(db *sql.DB) *ChildRunStartReservationStore {
	return &ChildRunStartReservationStore{db: db}
}

// Reserve inserts one record or accepts an exact replay. Reusing MemberID with
// different content is a durable identity conflict.
func (c *ChildRunStartReservationStore) Reserve(
	ctx context.Context,
	record ChildRunStartReservationRecord,
) error {
	if c == nil || c.db == nil {
		return errors.New("sqlite: child Run start reservation store is unavailable")
	}
	if err := record.validate(); err != nil {
		return err
	}
	result, err := conn(ctx, c.db).ExecContext(ctx, `
		INSERT INTO child_run_start_reservations(member_id, session_id, payload, created_at, state)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(member_id) DO NOTHING`,
		record.MemberID, record.SessionID, record.Payload, record.CreatedAt.UTC().UnixNano(), childRunStartReserved,
	)
	if err != nil {
		return fmt.Errorf("sqlite: reserve child Run start: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect child Run start reservation: %w", err)
	}
	if inserted == 1 {
		return nil
	}
	existing, _, err := c.load(ctx, record.MemberID)
	if err != nil {
		return err
	}
	if existing.SessionID != record.SessionID || !existing.CreatedAt.Equal(record.CreatedAt.UTC()) ||
		!bytes.Equal(existing.Payload, record.Payload) {
		return ErrChildRunStartReservationConflict
	}
	return nil
}

// Conclude advances an exact reservation from reserved to one conclusive
// state. changed is true only for the first conclusion. An exact replay of the
// same conclusion returns false; absence, different content, or a different
// prior conclusion is a conflict.
func (c *ChildRunStartReservationStore) Conclude(
	ctx context.Context,
	record ChildRunStartReservationRecord,
	conclusion ChildRunStartConclusion,
) (bool, error) {
	if c == nil || c.db == nil {
		return false, errors.New("sqlite: child Run start reservation store is unavailable")
	}
	if err := record.validate(); err != nil {
		return false, err
	}
	if !conclusion.valid() {
		return false, errors.New("sqlite: child Run start conclusion is invalid")
	}
	existing, state, err := c.load(ctx, record.MemberID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrChildRunStartReservationConflict
	}
	if err != nil {
		return false, err
	}
	if existing.SessionID != record.SessionID || !existing.CreatedAt.Equal(record.CreatedAt.UTC()) ||
		!bytes.Equal(existing.Payload, record.Payload) {
		return false, ErrChildRunStartReservationConflict
	}
	if state == string(conclusion) {
		return false, nil
	}
	if state != childRunStartReserved {
		return false, ErrChildRunStartReservationConflict
	}
	result, err := conn(ctx, c.db).ExecContext(ctx, `
		UPDATE child_run_start_reservations
		SET state = ?
		WHERE member_id = ? AND state = ?`, conclusion, record.MemberID, childRunStartReserved)
	if err != nil {
		return false, fmt.Errorf("sqlite: conclude child Run start reservation: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("sqlite: inspect child Run start conclusion: %w", err)
	}
	if changed != 1 {
		return false, ErrChildRunStartReservationConflict
	}
	return true, nil
}

// DeleteSession removes every callback receipt owned by one Session. Child
// reservations are meaningful only while that Session's root execution tree is
// live; root terminalization, rollback/restore, and Session deletion call this
// inside their existing write-set so no concluded or reserve-before-abort row
// outlives its owner.
func (c *ChildRunStartReservationStore) DeleteSession(
	ctx context.Context,
	sessionID string,
) error {
	if c == nil || c.db == nil {
		return errors.New("sqlite: child Run start reservation store is unavailable")
	}
	if strings.TrimSpace(sessionID) == "" || sessionID != strings.TrimSpace(sessionID) {
		return fmt.Errorf("%w: session ID", ErrInvalidChildRunStartReservation)
	}
	if _, err := conn(ctx, c.db).ExecContext(ctx,
		`DELETE FROM child_run_start_reservations WHERE session_id = ?`, sessionID,
	); err != nil {
		return fmt.Errorf("sqlite: delete Session child Run start reservations: %w", err)
	}
	return nil
}

// DeleteAll retires the callback ledger of the previous Runtime process during
// boot reconciliation. No executor callback survives a process boundary, even
// when the corresponding public Run is a coherent parked tree preserved for
// later restore.
func (c *ChildRunStartReservationStore) DeleteAll(ctx context.Context) error {
	if c == nil || c.db == nil {
		return errors.New("sqlite: child Run start reservation store is unavailable")
	}
	if _, err := conn(ctx, c.db).ExecContext(ctx,
		`DELETE FROM child_run_start_reservations`,
	); err != nil {
		return fmt.Errorf("sqlite: delete child Run start reservations: %w", err)
	}
	return nil
}

func (c *ChildRunStartReservationStore) load(
	ctx context.Context,
	memberID string,
) (ChildRunStartReservationRecord, string, error) {
	var record ChildRunStartReservationRecord
	var createdAt int64
	var state string
	err := conn(ctx, c.db).QueryRowContext(ctx, `
		SELECT member_id, session_id, payload, created_at, state
		FROM child_run_start_reservations
		WHERE member_id = ?`, memberID,
	).Scan(&record.MemberID, &record.SessionID, &record.Payload, &createdAt, &state)
	if err != nil {
		return ChildRunStartReservationRecord{}, "", err
	}
	record.CreatedAt = time.Unix(0, createdAt).UTC()
	if err := record.validate(); err != nil {
		return ChildRunStartReservationRecord{}, "", err
	}
	if state != childRunStartReserved && state != string(ChildRunStartConclusionStarted) &&
		state != string(ChildRunStartConclusionAborted) {
		return ChildRunStartReservationRecord{}, "", ErrChildRunStartReservationConflict
	}
	return record, state, nil
}
