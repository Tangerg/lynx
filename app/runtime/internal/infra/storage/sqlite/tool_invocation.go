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
	toolInvocationStarted    = "started"
	toolInvocationCompleted  = "completed"
	toolInvocationIncomplete = "incomplete"
)

// ToolInvocationStore is the SQLite operational journal for Tool attempts.
// Arguments and results deliberately stay out of this table: the canonical
// Transcript Item remains their single durable owner.
type ToolInvocationStore struct{ db *sql.DB }

func NewToolInvocationStore(db *sql.DB) *ToolInvocationStore {
	return &ToolInvocationStore{db: db}
}

func (store *ToolInvocationStore) StartToolInvocation(
	ctx context.Context,
	sessionID, runID, segmentID, callID, itemID string,
	startedAt time.Time,
) error {
	if err := validateToolInvocationIdentity(sessionID, runID, segmentID, callID, itemID); err != nil {
		return err
	}
	if startedAt.IsZero() {
		return errors.New("sqlite: Tool invocation start time is required")
	}
	_, err := conn(ctx, store.db).ExecContext(ctx,
		`INSERT INTO tool_invocations(
		   call_id, item_id, session_id, run_id, segment_id, state, started_at, finished_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 0)`,
		callID,
		itemID,
		sessionID,
		runID,
		segmentID,
		toolInvocationStarted,
		startedAt.UTC().UnixNano(),
	)
	if err != nil {
		return fmt.Errorf("sqlite: start Tool invocation %q: %w", callID, err)
	}
	return nil
}

func (store *ToolInvocationStore) CompleteToolInvocation(
	ctx context.Context,
	sessionID, runID, segmentID, callID, itemID string,
	startedAt, finishedAt time.Time,
) error {
	return store.finish(
		ctx, sessionID, runID, segmentID, callID, itemID,
		startedAt, finishedAt, toolInvocationCompleted,
	)
}

func (store *ToolInvocationStore) MarkToolInvocationIncomplete(
	ctx context.Context,
	sessionID, runID, segmentID, callID, itemID string,
	startedAt, finishedAt time.Time,
) error {
	return store.finish(
		ctx, sessionID, runID, segmentID, callID, itemID,
		startedAt, finishedAt, toolInvocationIncomplete,
	)
}

func (store *ToolInvocationStore) finish(
	ctx context.Context,
	sessionID, runID, segmentID, callID, itemID string,
	startedAt, finishedAt time.Time,
	state string,
) error {
	if err := validateToolInvocationIdentity(sessionID, runID, segmentID, callID, itemID); err != nil {
		return err
	}
	if startedAt.IsZero() || finishedAt.IsZero() {
		return errors.New("sqlite: terminal Tool invocation requires start and finish times")
	}
	if finishedAt.Before(startedAt) {
		return errors.New("sqlite: Tool invocation finish time precedes start time")
	}
	result, err := conn(ctx, store.db).ExecContext(ctx,
		`UPDATE tool_invocations
		    SET state = ?, finished_at = ?
		  WHERE call_id = ? AND item_id = ? AND session_id = ? AND run_id = ? AND segment_id = ?
		    AND state = ? AND started_at = ?`,
		state,
		finishedAt.UTC().UnixNano(),
		callID,
		itemID,
		sessionID,
		runID,
		segmentID,
		toolInvocationStarted,
		startedAt.UTC().UnixNano(),
	)
	if err != nil {
		return fmt.Errorf("sqlite: finish Tool invocation %q as %s: %w", callID, state, err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect Tool invocation %q transition: %w", callID, err)
	}
	if changed != 1 {
		return fmt.Errorf("sqlite: Tool invocation %q no longer owns its started transition", callID)
	}
	return nil
}

func validateToolInvocationIdentity(sessionID, runID, segmentID, callID, itemID string) error {
	for _, identity := range []struct {
		name  string
		value string
	}{
		{name: "session", value: sessionID},
		{name: "Run", value: runID},
		{name: "segment", value: segmentID},
		{name: "call", value: callID},
		{name: "Item", value: itemID},
	} {
		if strings.TrimSpace(identity.value) == "" || identity.value != strings.TrimSpace(identity.value) {
			return fmt.Errorf(
				"sqlite: Tool invocation %s ID is required without surrounding whitespace",
				identity.name,
			)
		}
	}
	return nil
}
