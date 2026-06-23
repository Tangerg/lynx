package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// TranscriptStore implements [transcript.Store] against a SQLite database — the
// durable Item history a session's items.list is served from. Two tables:
// history_items (append-only, ordered by an autoincrement seq) and
// history_runs (one row per run, UPSERT by run_id). Wire Item / RunRef
// payloads are stored as opaque JSON text; created_at / updated_at as
// unix nanos.
type TranscriptStore struct {
	db *sql.DB
}

var _ transcript.Store = (*TranscriptStore)(nil)

// NewTranscriptStore binds the SQLite history to db. db must have been
// opened via [Open] so the migration ran.
func NewTranscriptStore(db *sql.DB) *TranscriptStore {
	return &TranscriptStore{db: db}
}

func (s *TranscriptStore) AppendItem(ctx context.Context, it transcript.Item) error {
	if it.SessionID == "" {
		return errors.New("sqlite: history item sessionId is required")
	}
	if it.ItemID == "" {
		return errors.New("sqlite: history item itemId is required")
	}
	// item_id is UNIQUE: a re-appended item (e.g. a drainTools-produced
	// incomplete toolCall that a resumed HITL round later completes with
	// the same id) atomically updates in place, keeping its original seq —
	// items.list surfaces the latest state at the item's original position.
	_, err := conn(ctx, s.db).ExecContext(ctx,
		`INSERT INTO history_items(session_id, run_id, item_id, created_at, item)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(item_id) DO UPDATE SET
		   session_id = excluded.session_id,
		   run_id     = excluded.run_id,
		   item       = excluded.item`,
		it.SessionID, it.RunID, it.ItemID, it.CreatedAt.UnixNano(), string(it.Blob),
	)
	if err != nil {
		return fmt.Errorf("sqlite: append history item: %w", err)
	}
	return nil
}

func (s *TranscriptStore) PutRun(ctx context.Context, r transcript.Run) error {
	if r.SessionID == "" || r.RunID == "" {
		return errors.New("sqlite: history run sessionId/runId are required")
	}
	_, err := conn(ctx, s.db).ExecContext(ctx,
		`INSERT INTO history_runs(run_id, session_id, updated_at, run, message_mark)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(run_id) DO UPDATE SET
		   session_id   = excluded.session_id,
		   updated_at   = excluded.updated_at,
		   run          = excluded.run,
		   message_mark = excluded.message_mark`,
		r.RunID, r.SessionID, r.UpdatedAt.UnixNano(), string(r.Blob), r.Mark,
	)
	if err != nil {
		return fmt.Errorf("sqlite: put history run: %w", err)
	}
	return nil
}

// DeleteRun removes a run's record and all its items in one transaction
// (sessions.rollback drops the runs after the kept boundary). Unknown run /
// session is a no-op, not an error.
func (s *TranscriptStore) DeleteRun(ctx context.Context, sessionID, runID string) error {
	if sessionID == "" || runID == "" {
		return errors.New("sqlite: delete history run requires sessionId + runId")
	}
	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		q := conn(ctx, s.db)
		if _, err := q.ExecContext(ctx,
			`DELETE FROM history_items WHERE session_id = ? AND run_id = ?`, sessionID, runID,
		); err != nil {
			return fmt.Errorf("sqlite: delete run items: %w", err)
		}
		if _, err := q.ExecContext(ctx,
			`DELETE FROM history_runs WHERE run_id = ?`, runID,
		); err != nil {
			return fmt.Errorf("sqlite: delete run: %w", err)
		}
		return nil
	})
}

// DeleteSession removes every item + run for a session (sessions.rollback
// purges the subagent child sessions a dropped run spawned). Idempotent.
func (s *TranscriptStore) DeleteSession(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return errors.New("sqlite: delete history session requires sessionId")
	}
	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		q := conn(ctx, s.db)
		if _, err := q.ExecContext(ctx,
			`DELETE FROM history_items WHERE session_id = ?`, sessionID,
		); err != nil {
			return fmt.Errorf("sqlite: delete session items: %w", err)
		}
		if _, err := q.ExecContext(ctx,
			`DELETE FROM history_runs WHERE session_id = ?`, sessionID,
		); err != nil {
			return fmt.Errorf("sqlite: delete session runs: %w", err)
		}
		return nil
	})
}

func (s *TranscriptStore) List(ctx context.Context, sessionID string) ([]transcript.Item, []transcript.Run, error) {
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

// ListRuns returns just the session's run records (no item blobs) — the cheap
// path for usage aggregation, which only needs the runs.
func (s *TranscriptStore) ListRuns(ctx context.Context, sessionID string) ([]transcript.Run, error) {
	return s.listRuns(ctx, sessionID)
}

func (s *TranscriptStore) listItems(ctx context.Context, sessionID string) ([]transcript.Item, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT session_id, run_id, item_id, created_at, item
		 FROM history_items WHERE session_id = ? ORDER BY seq`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list history items: %w", err)
	}
	defer rows.Close()

	out := make([]transcript.Item, 0)
	for rows.Next() {
		var (
			it        transcript.Item
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

func (s *TranscriptStore) listRuns(ctx context.Context, sessionID string) ([]transcript.Run, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT session_id, run_id, updated_at, run, message_mark
		 FROM history_runs WHERE session_id = ? ORDER BY updated_at`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list history runs: %w", err)
	}
	defer rows.Close()

	out := make([]transcript.Run, 0)
	for rows.Next() {
		var (
			r         transcript.Run
			updatedNs int64
			blob      string
		)
		if err := rows.Scan(&r.SessionID, &r.RunID, &updatedNs, &blob, &r.Mark); err != nil {
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
