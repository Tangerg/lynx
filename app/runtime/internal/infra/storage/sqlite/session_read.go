package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// List returns user-facing sessions (roots and forks), newest-updated first.
// Internal subtask-delegation sessions ([session.KindSubtask]) are excluded so
// they never clutter the session list — query the lineage via [Children].
func (s *SessionStore) List(ctx context.Context) ([]session.Session, error) {
	rows, err := conn(ctx, s.db).QueryContext(ctx,
		`SELECT `+sessionColumns+` FROM sessions WHERE kind != ? ORDER BY favorite DESC, updated_at DESC, id DESC`,
		session.KindSubtask)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list sessions: %w", err)
	}
	defer rows.Close()

	out := make([]session.Session, 0)
	for rows.Next() {
		sess, err := rowToSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list sessions: %w", err)
	}
	return out, nil
}

// Exists reports whether a session row exists — the cheap existence check the
// goal driver uses to refuse a goal for a missing session and to sweep orphaned
// goals at boot, without decoding the whole aggregate.
func (s *SessionStore) Exists(ctx context.Context, id string) (bool, error) {
	var one int
	err := conn(ctx, s.db).QueryRowContext(ctx, `SELECT 1 FROM sessions WHERE id = ?`, id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("sqlite: session exists: %w", err)
	}
	return true, nil
}

func (s *SessionStore) Get(ctx context.Context, id string) (session.Session, error) {
	row := conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT `+sessionColumns+` FROM sessions WHERE id = ?`, id)
	sess, err := rowToSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return session.Session{}, session.ErrNotFound
	}
	if err != nil {
		return session.Session{}, fmt.Errorf("sqlite: get session: %w", err)
	}
	return sess, nil
}

// Children returns the sessions whose parent_id is parentID — the delegation /
// fork lineage under a session, newest-updated first. Includes KindSubtask
// children (which List hides).
func (s *SessionStore) Children(ctx context.Context, parentID string) ([]session.Session, error) {
	rows, err := conn(ctx, s.db).QueryContext(ctx,
		`SELECT `+sessionColumns+` FROM sessions WHERE parent_id = ? ORDER BY updated_at DESC`,
		parentID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list session children: %w", err)
	}
	defer rows.Close()

	out := make([]session.Session, 0)
	for rows.Next() {
		sess, err := rowToSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list session children: %w", err)
	}
	return out, nil
}
