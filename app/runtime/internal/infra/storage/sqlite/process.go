package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
)

// ProcessStore implements [core.ProcessStore] against SQLite. The Agent
// framework supplies complete snapshots and process-tree roots; this adapter
// chooses SQLite transactions, upserts, and recursive cleanup as its storage
// strategy.
type ProcessStore struct {
	db *sql.DB
}

var (
	_ core.ProcessStore  = (*ProcessStore)(nil)
	_ core.ProcessLister = (*ProcessStore)(nil)
)

// NewProcessStore binds the process store to a database opened via [Open].
func NewProcessStore(db *sql.DB) *ProcessStore {
	return &ProcessStore{db: db}
}

type encodedProcessSnapshot struct {
	id         string
	parentID   string
	data       []byte
	capturedAt int64
}

// Apply persists one complete process-snapshot change using the adapter's
// transaction strategy.
func (s *ProcessStore) Apply(ctx context.Context, change core.ProcessSnapshotChange) error {
	if err := change.Validate(); err != nil {
		return fmt.Errorf("sqlite: apply process snapshot change: %w", err)
	}

	var encoded []encodedProcessSnapshot
	if change.Tree != nil {
		encoded = make([]encodedProcessSnapshot, len(change.Tree.Snapshots))
		for index, snapshot := range change.Tree.Snapshots {
			data, err := json.Marshal(snapshot)
			if err != nil {
				return fmt.Errorf("sqlite: encode process snapshots[%d]: %w", index, err)
			}
			encoded[index] = encodedProcessSnapshot{
				id:         snapshot.ID,
				parentID:   snapshot.ParentID,
				data:       data,
				capturedAt: snapshot.CapturedAt.UnixNano(),
			}
		}
	}

	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		for index, snapshot := range encoded {
			if _, err := conn(ctx, s.db).ExecContext(ctx,
				`INSERT INTO process_snapshots(id, parent_id, snapshot, captured_at)
				 VALUES (?, ?, ?, ?)
				 ON CONFLICT(id) DO UPDATE SET
				 parent_id = excluded.parent_id,
				 snapshot = excluded.snapshot,
				 captured_at = excluded.captured_at`,
				snapshot.id, snapshot.parentID, string(snapshot.data), snapshot.capturedAt,
			); err != nil {
				return fmt.Errorf("sqlite: save process snapshots[%d] %q: %w", index, snapshot.id, err)
			}
		}
		for _, rootID := range change.DeleteRoots {
			if err := s.deleteTree(ctx, rootID); err != nil {
				return err
			}
		}
		return nil
	})
}

// Load returns the snapshot for id, or an error wrapping
// [core.ErrSnapshotNotFound] when the id is unknown.
func (s *ProcessStore) Load(ctx context.Context, id string) (core.ProcessSnapshot, error) {
	var data string
	err := conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT snapshot FROM process_snapshots WHERE id = ?`, id,
	).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return core.ProcessSnapshot{}, fmt.Errorf("sqlite: load %q: %w", id, core.ErrSnapshotNotFound)
	}
	if err != nil {
		return core.ProcessSnapshot{}, fmt.Errorf("sqlite: load snapshot: %w", err)
	}
	var snapshot core.ProcessSnapshot
	if err := json.Unmarshal([]byte(data), &snapshot); err != nil {
		return core.ProcessSnapshot{}, fmt.Errorf("sqlite: parse snapshot %q: %w: %w", id, core.ErrInvalidSnapshot, err)
	}
	if snapshot.ID != id {
		return core.ProcessSnapshot{}, fmt.Errorf("sqlite: snapshot ID %q does not match row %q: %w", snapshot.ID, id, core.ErrInvalidSnapshot)
	}
	return snapshot, nil
}

// List returns every stored process id, most-recently-captured first.
func (s *ProcessStore) List(ctx context.Context) ([]string, error) {
	rows, err := conn(ctx, s.db).QueryContext(ctx,
		`SELECT id FROM process_snapshots ORDER BY captured_at DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list snapshots: %w", err)
	}
	defer rows.Close()

	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("sqlite: scan snapshot id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list snapshots: %w", err)
	}
	return ids, nil
}

func (s *ProcessStore) deleteTree(ctx context.Context, rootID string) error {
	if _, err := conn(ctx, s.db).ExecContext(ctx,
		`WITH RECURSIVE process_tree(id) AS (
			SELECT id FROM process_snapshots WHERE id = ?
			UNION
			SELECT child.id
			FROM process_snapshots AS child
			JOIN process_tree AS parent ON child.parent_id = parent.id
		)
		DELETE FROM process_snapshots WHERE id IN (SELECT id FROM process_tree)`,
		rootID,
	); err != nil {
		return fmt.Errorf("sqlite: delete process tree %q: %w", rootID, err)
	}
	return nil
}
