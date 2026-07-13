package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
)

// ProcessStore implements [core.ProcessStore] against a SQLite database.
// The whole snapshot is marshaled to JSON and stored in one row keyed by
// process id; captured_at is broken out as an integer (unix nanos) so
// callers can order / prune by recency without unmarshaling. Update is
// UPSERT-style so first write and overwrite share one path — matching
// the engine's auto-snapshot, which Saves the same id on every tick.
type ProcessStore struct {
	db *sql.DB
}

var _ core.ProcessStore = (*ProcessStore)(nil)

// NewProcessStore binds the SQLite process-store to a database opened via
// [Open].
func NewProcessStore(db *sql.DB) *ProcessStore {
	return &ProcessStore{db: db}
}

// Save persists snapshot under its id, overwriting any existing row.
func (s *ProcessStore) Save(ctx context.Context, snapshot core.ProcessSnapshot) error {
	if snapshot.ID == "" {
		return errors.New("sqlite: snapshot id is required")
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("sqlite: marshal snapshot: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO process_snapshots(id, snapshot, captured_at) VALUES (?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   snapshot    = excluded.snapshot,
		   captured_at = excluded.captured_at`,
		snapshot.ID, string(data), snapshot.CapturedAt.UnixNano(),
	)
	if err != nil {
		return fmt.Errorf("sqlite: save snapshot: %w", err)
	}
	return nil
}

// Load returns the snapshot for id, or an error wrapping
// [core.ErrSnapshotNotFound] when the id is unknown.
func (s *ProcessStore) Load(ctx context.Context, id string) (core.ProcessSnapshot, error) {
	var data string
	err := s.db.QueryRowContext(ctx,
		`SELECT snapshot FROM process_snapshots WHERE id = ?`, id,
	).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return core.ProcessSnapshot{}, fmt.Errorf("sqlite: load %q: %w", id, core.ErrSnapshotNotFound)
	}
	if err != nil {
		return core.ProcessSnapshot{}, fmt.Errorf("sqlite: load snapshot: %w", err)
	}
	var snap core.ProcessSnapshot
	if err := json.Unmarshal([]byte(data), &snap); err != nil {
		return core.ProcessSnapshot{}, fmt.Errorf("sqlite: parse snapshot %q: %w", id, err)
	}
	return snap, nil
}

// Delete removes the snapshot for id. Idempotent — unknown id is not an
// error.
func (s *ProcessStore) Delete(ctx context.Context, id string) error {
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM process_snapshots WHERE id = ?`, id,
	); err != nil {
		return fmt.Errorf("sqlite: delete snapshot: %w", err)
	}
	return nil
}

// List returns every stored process id, most-recently-captured first.
func (s *ProcessStore) List(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM process_snapshots ORDER BY captured_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list snapshots: %w", err)
	}
	defer rows.Close()

	out := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("sqlite: scan snapshot id: %w", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list snapshots: %w", err)
	}
	return out, nil
}
