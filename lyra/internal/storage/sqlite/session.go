package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/Tangerg/lynx/lyra/internal/service/session"
)

// SessionService implements session.Service against a SQLite database.
// Mutations are single-row INSERT / UPDATE / DELETE — no rollback gymnastics
// needed compared to FileSessionService (which has to undo the in-memory
// repo when persist fails).
//
// All methods are safe for concurrent use; the underlying *sql.DB serializes
// writes when MaxOpenConns is 1 (see [Open]).
type SessionService struct {
	db *sql.DB
}

// NewSessionService wires the given *sql.DB to the session.Service surface.
// The DB must have been opened via [Open] so the migration ran.
func NewSessionService(db *sql.DB) *SessionService {
	return &SessionService{db: db}
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
	)
	if err := scanner.Scan(
		&s.ID, &s.Title, &s.Cwd, &s.ParentID,
		&startedAtNanos, &updatedAtNanos, &s.TurnCount, &metaJSON, &s.Model,
	); err != nil {
		return session.Session{}, err
	}
	s.StartedAt = time.Unix(0, startedAtNanos).UTC()
	s.UpdatedAt = time.Unix(0, updatedAtNanos).UTC()
	if metaJSON != "" && metaJSON != "{}" {
		if err := json.Unmarshal([]byte(metaJSON), &s.Metadata); err != nil {
			return session.Session{}, fmt.Errorf("sqlite: decode metadata: %w", err)
		}
	}
	return s, nil
}

// encodeMetadata marshals the metadata map; nil / empty maps become
// "{}" so the row's NOT NULL constraint stays satisfied.
func encodeMetadata(m map[string]string) (string, error) {
	if len(m) == 0 {
		return "{}", nil
	}
	data, err := json.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("sqlite: encode metadata: %w", err)
	}
	return string(data), nil
}

const sessionColumns = `id, title, cwd, parent_id, started_at, updated_at, turn_count, metadata, model`

// ------------------------------------------------------------------
// session.Service
// ------------------------------------------------------------------

func (s *SessionService) List(ctx context.Context) ([]session.Session, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+sessionColumns+` FROM sessions ORDER BY updated_at DESC`)
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

func (s *SessionService) Get(ctx context.Context, id string) (session.Session, error) {
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

func (s *SessionService) Create(ctx context.Context, title, cwd string) (session.Session, error) {
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

// Fork checks the parent exists and inserts the child in a single
// transaction so a concurrent Delete on the parent can't race against
// the fork.
func (s *SessionService) Fork(ctx context.Context, parentID, atMessageID string) (session.Session, error) {
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

// Delete is idempotent — deleting an unknown id is not an error
// (matches session.Service contract).
func (s *SessionService) Delete(ctx context.Context, id string) error {
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM sessions WHERE id = ?`, id,
	); err != nil {
		return fmt.Errorf("sqlite: delete session: %w", err)
	}
	return nil
}

// Touch refreshes UpdatedAt + bumps TurnCount in a single UPDATE. The bump is
// kept as atomic SQL (turn_count = turn_count + 1) rather than a
// load-modify-store on the entity, so concurrent turns can't lose a count.
// Lives off the public interface: it's implementation-detail bookkeeping the
// engine calls between turns. ErrNotFound for unknown id.
func (s *SessionService) Touch(ctx context.Context, id string) error {
	return s.updateByID(ctx, "touch session",
		`UPDATE sessions SET updated_at = ?, turn_count = turn_count + 1 WHERE id = ?`,
		time.Now().UTC().UnixNano(), id)
}

// SetModel records the session's current model + refreshes UpdatedAt in a
// single UPDATE (see [session.Service.SetModel]). ErrNotFound for unknown id.
func (s *SessionService) SetModel(ctx context.Context, id, model string) error {
	return s.updateByID(ctx, "set session model",
		`UPDATE sessions SET model = ?, updated_at = ? WHERE id = ?`,
		model, time.Now().UTC().UnixNano(), id)
}

// Rename updates the session's title + refreshes UpdatedAt in a single UPDATE
// (see [session.Service.Rename]). ErrNotFound for unknown id.
func (s *SessionService) Rename(ctx context.Context, id, title string) error {
	return s.updateByID(ctx, "rename session",
		`UPDATE sessions SET title = ?, updated_at = ? WHERE id = ?`,
		title, time.Now().UTC().UnixNano(), id)
}

// updateByID runs a single-row UPDATE keyed on session id and maps "no row
// matched" to session.ErrNotFound. op labels the operation for error wrapping
// (e.g. "rename session"). Shared by the Touch / SetModel / Rename field
// writes, which differ only in their SET clause and bound args.
func (s *SessionService) updateByID(ctx context.Context, op, query string, args ...any) error {
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
func (s *SessionService) insert(ctx context.Context, sess session.Session) error {
	return s.execInsert(ctx, s.db, sess)
}

// execInsert is shared by Create and Fork. The execer interface
// accepts either *sql.DB or *sql.Tx.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func (s *SessionService) execInsert(ctx context.Context, ex execer, sess session.Session) error {
	metaJSON, err := encodeMetadata(sess.Metadata)
	if err != nil {
		return err
	}
	_, err = ex.ExecContext(ctx,
		`INSERT INTO sessions(`+sessionColumns+`)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sess.ID, sess.Title, sess.Cwd, sess.ParentID,
		sess.StartedAt.UnixNano(), sess.UpdatedAt.UnixNano(),
		sess.TurnCount, metaJSON, sess.Model,
	)
	if err != nil {
		return fmt.Errorf("sqlite: insert session: %w", err)
	}
	return nil
}
