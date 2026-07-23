package sqlite

import (
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

// SaveSubtask persists the complete Agent runtime identity while enriching a
// new product session with the parent's title and cwd. Re-saving the same
// child updates audit/delegation-metadata fields; changing immutable identity or reusing
// its ID for a user-facing session fails closed.
func (s *SessionStore) SaveSubtask(ctx context.Context, subtask session.Subtask) (session.Session, error) {
	if err := subtask.Validate(); err != nil {
		return session.Session{}, err
	}

	var saved session.Session
	err := RunInTx(ctx, s.db, func(ctx context.Context) error {
		existing, err := s.Get(ctx, subtask.ID)
		switch {
		case err == nil:
			if !subtask.SameIdentity(existing) {
				return fmt.Errorf(
					"%w: ID %q is already bound to kind %q, parent %q, user %q, agent %q, started_at %s",
					session.ErrSubtaskConflict,
					subtask.ID,
					existing.Kind,
					existing.ParentID,
					existing.UserID,
					existing.AgentName,
					existing.StartedAt.Format(time.RFC3339Nano),
				)
			}
			saved = existing
			saved.UpdatedAt = subtask.UpdatedAt
			saved.DelegationMetadata = subtask.DelegationMetadata
			return s.updateSubtaskAudit(ctx, saved)
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
			return s.insert(ctx, saved)
		default:
			return err
		}
	})
	if err != nil {
		return session.Session{}, err
	}
	return saved, nil
}

func (s *SessionStore) updateSubtaskAudit(ctx context.Context, subtask session.Session) error {
	return s.updateByID(
		ctx,
		"save subtask",
		`UPDATE sessions SET updated_at = ?, delegation_metadata = ? WHERE id = ?`,
		subtask.UpdatedAt.UnixNano(),
		subtask.DelegationMetadata.String(),
		subtask.ID,
	)
}
