package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/lyra/internal/service/history"
)

// HistoryStore implements [history.Store] against a SQLite database — the
// durable Item history a session's items.list is served from. Two tables:
// history_items (append-only, ordered by an autoincrement seq) and
// history_runs (one row per run, UPSERT by run_id). Wire Item / RunRef
// payloads are stored as opaque JSON text; created_at / updated_at as
// unix nanos.
type HistoryStore struct {
	db *sql.DB
}

var _ history.Store = (*HistoryStore)(nil)

// NewHistoryStore binds the SQLite history to db. db must have been
// opened via [Open] so the migration ran.
func NewHistoryStore(db *sql.DB) *HistoryStore {
	return &HistoryStore{db: db}
}

func (s *HistoryStore) AppendItem(ctx context.Context, it history.Item) error {
	if it.SessionID == "" {
		return errors.New("sqlite: history item sessionId is required")
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO history_items(session_id, run_id, item_id, created_at, item)
		 VALUES (?, ?, ?, ?, ?)`,
		it.SessionID, it.RunID, it.ItemID, it.CreatedAt.UnixNano(), string(it.Blob),
	)
	if err != nil {
		return fmt.Errorf("sqlite: append history item: %w", err)
	}
	return nil
}

func (s *HistoryStore) PutRun(ctx context.Context, r history.Run) error {
	if r.SessionID == "" || r.RunID == "" {
		return errors.New("sqlite: history run sessionId/runId are required")
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO history_runs(run_id, session_id, updated_at, run)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(run_id) DO UPDATE SET
		   session_id = excluded.session_id,
		   updated_at = excluded.updated_at,
		   run        = excluded.run`,
		r.RunID, r.SessionID, r.UpdatedAt.UnixNano(), string(r.Blob),
	)
	if err != nil {
		return fmt.Errorf("sqlite: put history run: %w", err)
	}
	return nil
}

func (s *HistoryStore) List(ctx context.Context, sessionID string) ([]history.Item, []history.Run, error) {
	items, err := s.listItems(ctx, sessionID)
	if err != nil {
		return nil, nil, err
	}
	runs, err := s.listRuns(ctx, sessionID)
	if err != nil {
		return nil, nil, err
	}
	return items, runs, nil
}

func (s *HistoryStore) listItems(ctx context.Context, sessionID string) ([]history.Item, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT session_id, run_id, item_id, created_at, item
		 FROM history_items WHERE session_id = ? ORDER BY seq`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list history items: %w", err)
	}
	defer rows.Close()

	out := make([]history.Item, 0)
	for rows.Next() {
		var (
			it        history.Item
			createdNs int64
			blob      string
		)
		if err := rows.Scan(&it.SessionID, &it.RunID, &it.ItemID, &createdNs, &blob); err != nil {
			return nil, fmt.Errorf("sqlite: scan history item: %w", err)
		}
		it.CreatedAt = time.Unix(0, createdNs).UTC()
		it.Blob = []byte(blob)
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list history items: %w", err)
	}
	return out, nil
}

func (s *HistoryStore) listRuns(ctx context.Context, sessionID string) ([]history.Run, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT session_id, run_id, updated_at, run
		 FROM history_runs WHERE session_id = ? ORDER BY updated_at`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list history runs: %w", err)
	}
	defer rows.Close()

	out := make([]history.Run, 0)
	for rows.Next() {
		var (
			r         history.Run
			updatedNs int64
			blob      string
		)
		if err := rows.Scan(&r.SessionID, &r.RunID, &updatedNs, &blob); err != nil {
			return nil, fmt.Errorf("sqlite: scan history run: %w", err)
		}
		r.UpdatedAt = time.Unix(0, updatedNs).UTC()
		r.Blob = []byte(blob)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list history runs: %w", err)
	}
	return out, nil
}
