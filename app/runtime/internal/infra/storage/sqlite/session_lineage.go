package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// Fork checks the parent exists and inserts the child in a single transaction so
// a concurrent Delete on the parent can't race against the fork. Uses the
// re-entrant [RunInTx] + conn(ctx) so it joins the fork write-set's transaction
// (seed history + rename) rather than opening a second connection.
func (s *SessionStore) Fork(ctx context.Context, parentID string) (session.Session, error) {
	var child session.Session
	err := RunInTx(ctx, s.db, func(ctx context.Context) error {
		q := conn(ctx, s.db)
		parent, err := rowToSession(q.QueryRowContext(ctx,
			`SELECT `+sessionColumns+` FROM sessions WHERE id = ?`, parentID))
		if errors.Is(err, sql.ErrNoRows) {
			return session.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("sqlite: fork parent lookup: %w", err)
		}
		// The fork-derivation rule (title suffix, cwd inheritance, lineage) is a
		// Session invariant — the adapter only supplies the new ID and clock.
		child = parent.Fork(session.IDPrefix+uuid.NewString(), time.Now().UTC())
		return s.execInsert(ctx, q, child)
	})
	if err != nil {
		return session.Session{}, err
	}
	return child, nil
}

// SaveSubtask persists a delegated product session and its opaque agent-runtime
// continuation sidecar in one transaction. The product aggregate owns only
// lineage/presentation; Bootstrap owns encoding and interpreting agentSession.
// Re-saving updates audit time and continuation data; changing product identity
// or reusing an ID for a user-facing session fails closed.
func (s *SessionStore) SaveSubtask(ctx context.Context, subtask session.Subtask, agentSession []byte) (session.Session, error) {
	if err := subtask.Validate(); err != nil {
		return session.Session{}, err
	}
	if len(agentSession) == 0 {
		return session.Session{}, errors.New("sqlite: agent session state is required")
	}

	var saved session.Session
	err := RunInTx(ctx, s.db, func(ctx context.Context) error {
		existing, err := s.Get(ctx, subtask.ID)
		switch {
		case err == nil:
			if !subtask.SameIdentity(existing) {
				return fmt.Errorf(
					"%w: ID %q is already bound to kind %q, parent %q, started_at %s",
					session.ErrSubtaskConflict,
					subtask.ID,
					existing.Kind,
					existing.ParentID,
					existing.StartedAt.Format(time.RFC3339Nano),
				)
			}
			saved = existing
			saved.UpdatedAt = subtask.UpdatedAt
			return s.updateSubtaskAudit(ctx, saved, agentSession)
		case errors.Is(err, session.ErrNotFound):
			parent, parentErr := s.Get(ctx, subtask.ParentID)
			if errors.Is(parentErr, session.ErrNotFound) {
				parent = session.Session{ID: subtask.ParentID}
			} else if parentErr != nil {
				return parentErr
			}
			saved, err = parent.NewSubtask(subtask)
			if err != nil {
				return err
			}
			if err := s.insert(ctx, saved); err != nil {
				return err
			}
			return s.saveAgentSession(ctx, saved.ID, agentSession)
		default:
			return err
		}
	})
	if err != nil {
		return session.Session{}, err
	}
	return saved, nil
}

func (s *SessionStore) updateSubtaskAudit(ctx context.Context, subtask session.Session, agentSession []byte) error {
	if err := s.updateByID(
		ctx,
		"save subtask",
		`UPDATE sessions SET updated_at = ? WHERE id = ?`,
		subtask.UpdatedAt.UnixNano(),
		subtask.ID,
	); err != nil {
		return err
	}
	return s.saveAgentSession(ctx, subtask.ID, agentSession)
}

func (s *SessionStore) saveAgentSession(ctx context.Context, sessionID string, agentSession []byte) error {
	_, err := conn(ctx, s.db).ExecContext(ctx,
		`INSERT INTO agent_session_state(session_id, session_json) VALUES (?, ?)
		 ON CONFLICT(session_id) DO UPDATE SET session_json = excluded.session_json`,
		sessionID, agentSession,
	)
	if err != nil {
		return fmt.Errorf("sqlite: save agent session state: %w", err)
	}
	return nil
}

// LoadSubtask returns the product session plus the opaque continuation sidecar
// required by Bootstrap to restore an Agent core session. A product subtask
// without a sidecar is invalid — it cannot be resumed safely.
func (s *SessionStore) LoadSubtask(ctx context.Context, id string) (session.Session, []byte, error) {
	stored, err := s.Get(ctx, id)
	if err != nil {
		return session.Session{}, nil, err
	}
	if stored.Kind != session.KindSubtask {
		return session.Session{}, nil, fmt.Errorf("%w: session %q has kind %q", session.ErrSubtaskConflict, id, stored.Kind)
	}
	var encoded []byte
	err = conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT session_json FROM agent_session_state WHERE session_id = ?`, id,
	).Scan(&encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return session.Session{}, nil, fmt.Errorf("%w: subtask %q has no continuation state", session.ErrSubtaskConflict, id)
	}
	if err != nil {
		return session.Session{}, nil, fmt.Errorf("sqlite: load agent session state: %w", err)
	}
	return stored, bytes.Clone(encoded), nil
}
