package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	modelInvocationStarted   = "started"
	modelInvocationCompleted = "completed"
	modelInvocationFailed    = "failed"
	modelInvocationUnknown   = "unknown"
)

// ModelInvocationStore is the SQLite operational journal for provider-call
// attempts. Semantic messages stay in history_items and accounting stays on the
// Run row; this table records only whether a crossed provider boundary reached a
// provable terminal observation.
type ModelInvocationStore struct{ db *sql.DB }

func NewModelInvocationStore(db *sql.DB) *ModelInvocationStore {
	return &ModelInvocationStore{db: db}
}

// ListStartedModelInvocations yields every provider attempt still owned by a
// pre-crash process. The callback keeps recovery application types out of the
// SQLite package while preserving an exact boot snapshot for its write-set.
func (m *ModelInvocationStore) ListStartedModelInvocations(
	ctx context.Context,
	yield func(sessionID, runID, segmentID, callID string, startedAt time.Time) error,
) error {
	if yield == nil {
		return errors.New("sqlite: model invocation reader is required")
	}
	rows, err := conn(ctx, m.db).QueryContext(ctx, `
		SELECT session_id, run_id, segment_id, call_id, started_at
		  FROM model_invocations
		 WHERE state = ?
		 ORDER BY session_id, run_id, segment_id, call_id
	`, modelInvocationStarted)
	if err != nil {
		return fmt.Errorf("sqlite: list started model invocations: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var sessionID, runID, segmentID, callID string
		var startedAt int64
		if err := rows.Scan(&sessionID, &runID, &segmentID, &callID, &startedAt); err != nil {
			return fmt.Errorf("sqlite: scan started model invocation: %w", err)
		}
		if err := yield(sessionID, runID, segmentID, callID, time.Unix(0, startedAt).UTC()); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("sqlite: iterate started model invocations: %w", err)
	}
	return nil
}

func (m *ModelInvocationStore) StartModelInvocation(
	ctx context.Context,
	sessionID, runID, segmentID, callID string,
	startedAt time.Time,
) error {
	if err := validateModelInvocationIdentity(sessionID, runID, segmentID, callID); err != nil {
		return err
	}
	if startedAt.IsZero() {
		return errors.New("sqlite: model invocation start time is required")
	}
	_, err := conn(ctx, m.db).ExecContext(ctx,
		`INSERT INTO model_invocations(
		   call_id, session_id, run_id, segment_id, state, started_at, finished_at)
		 VALUES (?, ?, ?, ?, ?, ?, 0)`,
		callID,
		sessionID,
		runID,
		segmentID,
		modelInvocationStarted,
		startedAt.UTC().UnixNano(),
	)
	if err != nil {
		return fmt.Errorf("sqlite: start model invocation %q: %w", callID, err)
	}
	return nil
}

func (m *ModelInvocationStore) CompleteModelInvocation(
	ctx context.Context,
	sessionID, runID, segmentID, callID string,
	startedAt, finishedAt time.Time,
) error {
	return m.finish(
		ctx, sessionID, runID, segmentID, callID,
		startedAt, finishedAt, modelInvocationCompleted,
	)
}

func (m *ModelInvocationStore) FailModelInvocation(
	ctx context.Context,
	sessionID, runID, segmentID, callID string,
	startedAt, finishedAt time.Time,
) error {
	return m.finish(
		ctx, sessionID, runID, segmentID, callID,
		startedAt, finishedAt, modelInvocationFailed,
	)
}

func (m *ModelInvocationStore) MarkModelInvocationUnknown(
	ctx context.Context,
	sessionID, runID, segmentID, callID string,
	startedAt, finishedAt time.Time,
) error {
	return m.finish(
		ctx, sessionID, runID, segmentID, callID,
		startedAt, finishedAt, modelInvocationUnknown,
	)
}

func (m *ModelInvocationStore) finish(
	ctx context.Context,
	sessionID, runID, segmentID, callID string,
	startedAt, finishedAt time.Time,
	state string,
) error {
	if err := validateModelInvocationIdentity(sessionID, runID, segmentID, callID); err != nil {
		return err
	}
	if startedAt.IsZero() || finishedAt.IsZero() {
		return errors.New("sqlite: terminal model invocation requires start and finish times")
	}
	if finishedAt.Before(startedAt) {
		return errors.New("sqlite: model invocation finish time precedes start time")
	}
	result, err := conn(ctx, m.db).ExecContext(ctx,
		`UPDATE model_invocations
		    SET state = ?, finished_at = ?
		  WHERE call_id = ? AND session_id = ? AND run_id = ? AND segment_id = ?
		    AND state = ? AND started_at = ?`,
		state,
		finishedAt.UTC().UnixNano(),
		callID,
		sessionID,
		runID,
		segmentID,
		modelInvocationStarted,
		startedAt.UTC().UnixNano(),
	)
	if err != nil {
		return fmt.Errorf("sqlite: finish model invocation %q as %s: %w", callID, state, err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect model invocation %q transition: %w", callID, err)
	}
	if changed != 1 {
		return fmt.Errorf(
			"sqlite: model invocation %q no longer owns its started transition",
			callID,
		)
	}
	return nil
}

func validateModelInvocationIdentity(sessionID, runID, segmentID, callID string) error {
	for _, identity := range []struct {
		name  string
		value string
	}{
		{name: "session", value: sessionID},
		{name: "Run", value: runID},
		{name: "segment", value: segmentID},
		{name: "call", value: callID},
	} {
		name, value := identity.name, identity.value
		if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) {
			return fmt.Errorf("sqlite: model invocation %s ID is required without surrounding whitespace", name)
		}
	}
	return nil
}
