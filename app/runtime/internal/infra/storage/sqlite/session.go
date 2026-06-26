package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// SessionStore implements session.Service against a SQLite database.
// Mutations are single-row INSERT / UPDATE / DELETE, so each operation is
// atomic on its own — no multi-step rollback handling needed.
//
// All methods are safe for concurrent use; the underlying *sql.DB serializes
// writes when MaxOpenConns is 1 (see [Open]).
type SessionStore struct {
	db *sql.DB
}

var _ session.Service = (*SessionStore)(nil)

// NewSessionStore wires the given *sql.DB to the session.Service surface.
// The DB must have been opened via [Open] so the migration ran.
func NewSessionStore(db *sql.DB) *SessionStore {
	return &SessionStore{db: db}
}

// rowToSession decodes one DB row into a session.Session. metadata is
// stored as a JSON blob; an empty / NULL value maps to a nil map.
func rowToSession(scanner interface {
	Scan(dest ...any) error
}) (session.Session, error) {
	var (
		s              session.Session
		startedAtNanos int64
		updatedAtNanos int64
		metaJSON       string
		favoriteInt    int64
	)
	if err := scanner.Scan(
		&s.ID, &s.Title, &s.Cwd, &s.ParentID,
		&startedAtNanos, &updatedAtNanos, &metaJSON, &s.Model, &s.Kind, &favoriteInt,
	); err != nil {
		return session.Session{}, err
	}
	s.StartedAt = time.Unix(0, startedAtNanos).UTC()
	s.UpdatedAt = time.Unix(0, updatedAtNanos).UTC()
	s.Favorite = favoriteInt != 0
	if metaJSON != "" && metaJSON != "{}" {
		if err := json.Unmarshal([]byte(metaJSON), &s.Metadata); err != nil {
			return session.Session{}, fmt.Errorf("sqlite: decode metadata: %w", err)
		}
	}
	return s, nil
}

// encodeMetadata marshals the metadata map; nil / empty maps become
// "{}" so the row's NOT NULL constraint stays satisfied.
func encodeMetadata(m map[string]any) (string, error) {
	if len(m) == 0 {
		return "{}", nil
	}
	data, err := json.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("sqlite: encode metadata: %w", err)
	}
	return string(data), nil
}

const sessionColumns = `id, title, cwd, parent_id, started_at, updated_at, metadata, model, kind, favorite`

// ------------------------------------------------------------------
// session.Service
// ------------------------------------------------------------------

// List returns user-facing sessions (roots and forks), newest-updated first.
// Internal subtask-delegation sessions ([session.KindSubtask]) are excluded so
// they never clutter the session list — query the lineage via [Children].
func (s *SessionStore) List(ctx context.Context) ([]session.Session, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+sessionColumns+` FROM sessions WHERE kind != ? ORDER BY updated_at DESC`,
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

func (s *SessionStore) Get(ctx context.Context, id string) (session.Session, error) {
	row := s.db.QueryRowContext(ctx,
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
// any existing row with that id (restore semantics). See
// [session.Service.Restore].
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

// Fork checks the parent exists and inserts the child in a single
// transaction so a concurrent Delete on the parent can't race against
// the fork.
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

// CreateSubtask records an internal delegation session under the
// caller-supplied id (the agent runtime's child conversation id), linked to
// parentID and marked [session.KindSubtask]. It inherits the parent's working
// directory and derives a title from it. Idempotent: a session already present
// under id is returned unchanged, so a re-driven spawn doesn't error.
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

// Children returns the sessions whose parent_id is parentID — the
// delegation / fork lineage under a session, newest-updated first. Includes
// KindSubtask children (which List hides).
func (s *SessionStore) Children(ctx context.Context, parentID string) ([]session.Session, error) {
	rows, err := s.db.QueryContext(ctx,
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

// Delete is idempotent — deleting an unknown id is not an error
// (matches session.Service contract).
func (s *SessionStore) Delete(ctx context.Context, id string) error {
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM sessions WHERE id = ?`, id,
	); err != nil {
		return fmt.Errorf("sqlite: delete session: %w", err)
	}
	return nil
}

// SetModel records the session's current model + refreshes UpdatedAt in a
// single UPDATE (see [session.Service.SetModel]). ErrNotFound for unknown id.
func (s *SessionStore) SetModel(ctx context.Context, id, model string) error {
	return s.updateByID(ctx, "set session model",
		`UPDATE sessions SET model = ?, updated_at = ? WHERE id = ?`,
		model, time.Now().UTC().UnixNano(), id)
}

// Rename updates the session's title + refreshes UpdatedAt in a single UPDATE
// (see [session.Service.Rename]). ErrNotFound for unknown id.
func (s *SessionStore) Rename(ctx context.Context, id, title string) error {
	return s.updateByID(ctx, "rename session",
		`UPDATE sessions SET title = ?, updated_at = ? WHERE id = ?`,
		title, time.Now().UTC().UnixNano(), id)
}

// RenameIfUntitled sets the title only on a session that has none, atomically
// (see [session.Service.RenameIfUntitled]). The WHERE guard collapses the
// titler's check-and-set into one statement so a concurrent user rename can't be
// clobbered across the async title generation. 0 rows affected (already titled
// or unknown id) is a no-op success, NOT ErrNotFound — so it can't use
// updateByID.
func (s *SessionStore) RenameIfUntitled(ctx context.Context, id, title string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET title = ?, updated_at = ? WHERE id = ? AND title = ''`,
		title, time.Now().UTC().UnixNano(), id)
	if err != nil {
		return fmt.Errorf("sqlite: rename-if-untitled session: %w", err)
	}
	return nil
}

// SetCwd relocates the session (see [session.Service.SetCwd]) + refreshes
// UpdatedAt in a single UPDATE. ErrNotFound for unknown id.
func (s *SessionStore) SetCwd(ctx context.Context, id, cwd string) error {
	return s.updateByID(ctx, "relocate session",
		`UPDATE sessions SET cwd = ?, updated_at = ? WHERE id = ?`,
		cwd, time.Now().UTC().UnixNano(), id)
}

// SetMetadata full-replaces the session's metadata (see
// [session.Service.SetMetadata]) + refreshes UpdatedAt. ErrNotFound for
// unknown id.
func (s *SessionStore) SetMetadata(ctx context.Context, id string, meta map[string]any) error {
	metaJSON, err := encodeMetadata(meta)
	if err != nil {
		return err
	}
	return s.updateByID(ctx, "set session metadata",
		`UPDATE sessions SET metadata = ?, updated_at = ? WHERE id = ?`,
		metaJSON, time.Now().UTC().UnixNano(), id)
}

// SetFavorite pins / unpins the session (see [session.Service.SetFavorite]) +
// refreshes UpdatedAt. ErrNotFound for unknown id.
func (s *SessionStore) SetFavorite(ctx context.Context, id string, favorite bool) error {
	return s.updateByID(ctx, "set session favorite",
		`UPDATE sessions SET favorite = ?, updated_at = ? WHERE id = ?`,
		favorite, time.Now().UTC().UnixNano(), id)
}

// updateByID runs a single-row UPDATE keyed on session id and maps "no row
// matched" to session.ErrNotFound. op labels the operation for error wrapping
// (e.g. "rename session"). Shared by the SetModel / Rename field writes,
// which differ only in their SET clause and bound args.
func (s *SessionStore) updateByID(ctx context.Context, op, query string, args ...any) error {
	res, err := s.db.ExecContext(ctx, query, args...)
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

// insert is the one-shot path used by Create. Fork goes through
// execInsert so it can run inside its own transaction.
func (s *SessionStore) insert(ctx context.Context, sess session.Session) error {
	return s.execInsert(ctx, s.db, sess)
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
