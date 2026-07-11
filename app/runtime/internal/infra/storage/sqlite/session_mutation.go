package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

func (s *SessionStore) Create(ctx context.Context, title, cwd string) (session.Session, error) {
	now := time.Now().UTC()
	sess := session.Session{
		ID:        session.IDPrefix + uuid.NewString(),
		Title:     title,
		Cwd:       cwd,
		StartedAt: now,
		UpdatedAt: now,
	}
	if err := s.insert(ctx, sess); err != nil {
		return session.Session{}, err
	}
	return sess, nil
}

// Restore upserts a session row verbatim (INSERT OR REPLACE) — the write side
// of sessions.import. It preserves the supplied id and all fields, overwriting
// any existing row with that id (restore semantics).
func (s *SessionStore) Restore(ctx context.Context, sess session.Session) error {
	metaJSON, err := encodeMetadata(sess.Metadata)
	if err != nil {
		return err
	}
	_, err = conn(ctx, s.db).ExecContext(ctx,
		`INSERT OR REPLACE INTO sessions(`+sessionColumns+`)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sess.ID, sess.Title, sess.Cwd, sess.ParentID,
		sess.StartedAt.UnixNano(), sess.UpdatedAt.UnixNano(),
		metaJSON, sess.Model, sess.Kind, sess.Favorite,
	)
	if err != nil {
		return fmt.Errorf("sqlite: restore session: %w", err)
	}
	return nil
}

// Delete is idempotent — deleting an unknown id is not an error. Uses conn(ctx)
// so it joins the delete-cascade transaction rather than opening a second
// connection (which deadlocks under the single-connection pool).
func (s *SessionStore) Delete(ctx context.Context, id string) error {
	if _, err := conn(ctx, s.db).ExecContext(ctx,
		`DELETE FROM sessions WHERE id = ?`, id,
	); err != nil {
		return fmt.Errorf("sqlite: delete session: %w", err)
	}
	return nil
}

// SetModel records the session's current model + refreshes UpdatedAt in a
// single UPDATE. ErrNotFound for unknown id.
func (s *SessionStore) SetModel(ctx context.Context, id, model string) error {
	return s.updateByID(ctx, "set session model",
		`UPDATE sessions SET model = ?, updated_at = ? WHERE id = ?`,
		model, time.Now().UTC().UnixNano(), id)
}

// Rename updates the session's title + refreshes UpdatedAt in a single UPDATE.
// ErrNotFound for unknown id.
func (s *SessionStore) Rename(ctx context.Context, id, title string) error {
	return s.updateByID(ctx, "rename session",
		`UPDATE sessions SET title = ?, updated_at = ? WHERE id = ?`,
		title, time.Now().UTC().UnixNano(), id)
}

// RenameIfUntitled sets the title only on a session that has none, atomically.
// The WHERE guard collapses the titler's check-and-set into one statement so a
// concurrent user rename can't be clobbered across the async title generation.
// 0 rows affected (already titled or unknown id) is a no-op success, NOT
// ErrNotFound — so it can't use updateByID.
func (s *SessionStore) RenameIfUntitled(ctx context.Context, id, title string) error {
	_, err := conn(ctx, s.db).ExecContext(ctx,
		`UPDATE sessions SET title = ?, updated_at = ? WHERE id = ? AND title = ''`,
		title, time.Now().UTC().UnixNano(), id)
	if err != nil {
		return fmt.Errorf("sqlite: rename-if-untitled session: %w", err)
	}
	return nil
}

// SetCwd relocates the session + refreshes UpdatedAt in a single UPDATE.
// ErrNotFound for unknown id.
func (s *SessionStore) SetCwd(ctx context.Context, id, cwd string) error {
	return s.updateByID(ctx, "relocate session",
		`UPDATE sessions SET cwd = ?, updated_at = ? WHERE id = ?`,
		cwd, time.Now().UTC().UnixNano(), id)
}

// SetMetadata full-replaces the session's metadata + refreshes UpdatedAt.
// ErrNotFound for unknown id.
func (s *SessionStore) SetMetadata(ctx context.Context, id string, meta map[string]any) error {
	metaJSON, err := encodeMetadata(meta)
	if err != nil {
		return err
	}
	return s.updateByID(ctx, "set session metadata",
		`UPDATE sessions SET metadata = ?, updated_at = ? WHERE id = ?`,
		metaJSON, time.Now().UTC().UnixNano(), id)
}

// SetFavorite pins / unpins the session + refreshes UpdatedAt. ErrNotFound for
// unknown id.
func (s *SessionStore) SetFavorite(ctx context.Context, id string, favorite bool) error {
	return s.updateByID(ctx, "set session favorite",
		`UPDATE sessions SET favorite = ?, updated_at = ? WHERE id = ?`,
		favorite, time.Now().UTC().UnixNano(), id)
}

// updateByID runs a single-row UPDATE keyed on session id and maps "no row
// matched" to session.ErrNotFound. op labels the operation for error wrapping
// (e.g. "rename session"). Shared by the SetModel / Rename field writes, which
// differ only in their SET clause and bound args.
func (s *SessionStore) updateByID(ctx context.Context, op, query string, args ...any) error {
	res, err := conn(ctx, s.db).ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("sqlite: %s: %w", op, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: %s: %w", op, err)
	}
	if n == 0 {
		return session.ErrNotFound
	}
	return nil
}

// insert is the one-shot path used by Create; conn(ctx) lets it join an ambient
// transaction (else it uses the pool directly). Fork goes through execInsert too.
func (s *SessionStore) insert(ctx context.Context, sess session.Session) error {
	return s.execInsert(ctx, conn(ctx, s.db), sess)
}

// execInsert is shared by Create and Fork; the shared [execer] (see tx.go)
// accepts either *sql.DB or *sql.Tx.
func (s *SessionStore) execInsert(ctx context.Context, ex execer, sess session.Session) error {
	metaJSON, err := encodeMetadata(sess.Metadata)
	if err != nil {
		return err
	}
	_, err = ex.ExecContext(ctx,
		`INSERT INTO sessions(`+sessionColumns+`)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sess.ID, sess.Title, sess.Cwd, sess.ParentID,
		sess.StartedAt.UnixNano(), sess.UpdatedAt.UnixNano(),
		metaJSON, sess.Model, sess.Kind, sess.Favorite,
	)
	if err != nil {
		return fmt.Errorf("sqlite: insert session: %w", err)
	}
	return nil
}
