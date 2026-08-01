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
