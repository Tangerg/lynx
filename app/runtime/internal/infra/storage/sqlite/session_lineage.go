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

// Fork checks the parent exists and inserts the child in a single transaction
// so a concurrent Delete on the parent can't race against the fork.
func (s *SessionStore) Fork(ctx context.Context, parentID, atMessageID string) (session.Session, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return session.Session{}, fmt.Errorf("sqlite: begin fork tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // commit overrides; rollback on early return

	parent, err := rowToSession(tx.QueryRowContext(ctx,
		`SELECT `+sessionColumns+` FROM sessions WHERE id = ?`, parentID))
	if errors.Is(err, sql.ErrNoRows) {
		return session.Session{}, session.ErrNotFound
	}
	if err != nil {
		return session.Session{}, fmt.Errorf("sqlite: fork parent lookup: %w", err)
	}

	// The fork-derivation rule (title suffix, cwd inheritance, branch-point
	// metadata) is a Session invariant — the adapter only supplies the new id
	// and the clock.
	child := parent.Fork(session.IDPrefix+uuid.NewString(), atMessageID, time.Now().UTC())
	if err := s.execInsert(ctx, tx, child); err != nil {
		return session.Session{}, err
	}
	if err := tx.Commit(); err != nil {
		return session.Session{}, fmt.Errorf("sqlite: commit fork: %w", err)
	}
	return child, nil
}

// CreateSubtask records an internal delegation session under the caller-supplied
// id (the agent runtime's child conversation id), linked to parentID and marked
// [session.KindSubtask]. It inherits the parent's working directory and derives
// a title from it. Idempotent: a session already present under id is returned
// unchanged, so a re-driven spawn doesn't error.
func (s *SessionStore) CreateSubtask(ctx context.Context, id, parentID string) (session.Session, error) {
	if existing, err := s.Get(ctx, id); err == nil {
		return existing, nil
	} else if !errors.Is(err, session.ErrNotFound) {
		return session.Session{}, err
	}

	// The subtask-derivation rule (title suffix, cwd inheritance, KindSubtask
	// marker) is a Session invariant — the adapter only supplies the new id and
	// the clock. A missing parent is passed as an id-only Session, which yields
	// the untitled-parent form.
	parent, err := s.Get(ctx, parentID)
	if errors.Is(err, session.ErrNotFound) {
		parent = session.Session{ID: parentID}
	} else if err != nil {
		return session.Session{}, err
	}

	child := parent.NewSubtask(id, time.Now().UTC())
	if err := s.insert(ctx, child); err != nil {
		return session.Session{}, err
	}
	return child, nil
}
