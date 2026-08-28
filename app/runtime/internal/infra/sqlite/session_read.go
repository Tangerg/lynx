package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Tangerg/scope/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/scope/app/runtime/internal/domain/session"
)

// List returns user-facing sessions (roots and forks), newest-updated first.
func (s *SessionStore) List(ctx context.Context) ([]session.Session, error) {
	return s.ListPage(ctx, false, 0, "", 0)
}

// ListPage returns user-facing sessions in list order, bounded by the query. The
// anchor is the full sort key a previous page ended at — pinned state, then
// update time, then id — because the earlier components tie freely and a partial
// bound would drop or repeat rows at a page boundary.
//
// Paging here rather than after the fact matters more than for most reads: each
// session's view is resolved against the filesystem and the live-run registry, so
// slicing a fully-resolved list did that work for every session to return one
// page of them.
func (s *SessionStore) ListPage(ctx context.Context, afterFavorite bool, afterUpdatedAt int64, afterID string, limit int) ([]session.Session, error) {
	query := `SELECT ` + sessionColumns + ` FROM sessions`
	var args []any
	if afterUpdatedAt > 0 || afterID != "" {
		favorite := 0
		if afterFavorite {
			favorite = 1
		}
		query += ` WHERE (favorite < ?
			OR (favorite = ? AND updated_at < ?)
			OR (favorite = ? AND updated_at = ? AND id < ?))`
		args = append(args, favorite, favorite, afterUpdatedAt, favorite, afterUpdatedAt, afterID)
	}
	query += ` ORDER BY favorite DESC, updated_at DESC, id DESC`
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := conn(ctx, s.db).QueryContext(ctx, query, args...)
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

// ModelSelection returns the Session's exact durable model policy without
// decoding unrelated aggregate state. Goal admission uses the boolean as its
// existence fact and freezes the returned selection when no override was sent.
func (s *SessionStore) ModelSelection(ctx context.Context, id string) (modelref.Selection, bool, error) {
	var provider, model, reasoningEffort string
	err := conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT provider, model, reasoning_effort FROM sessions WHERE id = ?`, id,
	).Scan(&provider, &model, &reasoningEffort)
	if errors.Is(err, sql.ErrNoRows) {
		return modelref.Selection{}, false, nil
	}
	if err != nil {
		return modelref.Selection{}, false, fmt.Errorf("sqlite: read Session model selection: %w", err)
	}
	selection, err := modelref.NewWithReasoningEffort(provider, model, reasoningEffort)
	if err != nil {
		return modelref.Selection{}, false, fmt.Errorf("sqlite: decode Session model selection: %w", err)
	}
	return selection, true, nil
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
