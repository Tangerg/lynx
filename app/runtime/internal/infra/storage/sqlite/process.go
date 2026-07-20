package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/Tangerg/lynx/agent/core"
)

// ProcessStore implements [core.ProcessStore] against a SQLite database.
// The whole snapshot is marshaled to JSON and stored in one row keyed by
// process id; captured_at is broken out as an integer (unix nanos) so
// callers can order / prune by recency without unmarshaling. Creation and
// updates use atomic revision compare-and-swap statements; the store never
// blindly overwrites a newer snapshot.
type ProcessStore struct {
	db *sql.DB
}

var _ core.ProcessStore = (*ProcessStore)(nil)
var _ core.SnapshotBatchWriter = (*ProcessStore)(nil)

// NewProcessStore binds the SQLite process-store to a database opened via
// [Open].
func NewProcessStore(db *sql.DB) *ProcessStore {
	return &ProcessStore{db: db}
}

// Save commits one compare-and-swap revision.
func (s *ProcessStore) Save(ctx context.Context, snapshot core.ProcessSnapshot, expectedRevision uint64) (uint64, error) {
	revisions, err := s.SaveBatch(ctx, []core.SnapshotWrite{{
		Snapshot:         snapshot,
		ExpectedRevision: expectedRevision,
	}})
	if err != nil {
		return 0, err
	}
	return revisions[0], nil
}

// SaveBatch commits every compare-and-swap revision in one transaction.
func (s *ProcessStore) SaveBatch(ctx context.Context, writes []core.SnapshotWrite) ([]uint64, error) {
	prepared, err := prepareSnapshotWrites(writes)
	if err != nil {
		return nil, err
	}
	if len(prepared) == 0 {
		return nil, nil
	}

	err = RunInTx(ctx, s.db, func(ctx context.Context) error {
		for index := range prepared {
			if err := s.savePreparedSnapshot(ctx, prepared[index]); err != nil {
				return fmt.Errorf("sqlite: save snapshot batch write[%d]: %w", index, err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	revisions := make([]uint64, len(prepared))
	for index, write := range prepared {
		revisions[index] = write.expectedRevision + 1
	}
	return revisions, nil
}

type preparedSnapshotWrite struct {
	snapshot         core.ProcessSnapshot
	expectedRevision uint64
	data             []byte
}

func prepareSnapshotWrites(writes []core.SnapshotWrite) ([]preparedSnapshotWrite, error) {
	prepared := make([]preparedSnapshotWrite, len(writes))
	seen := make(map[string]struct{}, len(writes))
	for index, write := range writes {
		if write.Snapshot.Revision != write.ExpectedRevision {
			return nil, fmt.Errorf("sqlite: snapshot batch write[%d]: %w: snapshot revision %d does not match expected %d",
				index, core.ErrInvalidSnapshot, write.Snapshot.Revision, write.ExpectedRevision)
		}
		if write.ExpectedRevision == math.MaxUint64 {
			return nil, fmt.Errorf("sqlite: snapshot batch write[%d]: %w: revision is exhausted", index, core.ErrInvalidSnapshot)
		}
		if err := write.Snapshot.Validate(); err != nil {
			return nil, fmt.Errorf("sqlite: snapshot batch write[%d]: %w", index, err)
		}
		if _, duplicate := seen[write.Snapshot.ID]; duplicate {
			return nil, fmt.Errorf("sqlite: snapshot batch write[%d]: %w: duplicate process ID %q", index, core.ErrInvalidSnapshot, write.Snapshot.ID)
		}
		seen[write.Snapshot.ID] = struct{}{}

		candidate := write.Snapshot
		candidate.Revision = write.ExpectedRevision + 1
		data, err := json.Marshal(candidate)
		if err != nil {
			return nil, fmt.Errorf("sqlite: snapshot batch write[%d]: marshal: %w", index, err)
		}
		prepared[index] = preparedSnapshotWrite{
			snapshot:         candidate,
			expectedRevision: write.ExpectedRevision,
			data:             data,
		}
	}
	return prepared, nil
}

func (s *ProcessStore) savePreparedSnapshot(ctx context.Context, write preparedSnapshotWrite) error {
	var result sql.Result
	var err error
	if write.expectedRevision == 0 {
		result, err = conn(ctx, s.db).ExecContext(ctx,
			`INSERT INTO process_snapshots(id, revision, snapshot, captured_at)
			 VALUES (?, 1, ?, ?)
			 ON CONFLICT(id) DO NOTHING`,
			write.snapshot.ID, string(write.data), write.snapshot.CapturedAt.UnixNano(),
		)
	} else {
		result, err = conn(ctx, s.db).ExecContext(ctx,
			`UPDATE process_snapshots
			 SET revision = ?, snapshot = ?, captured_at = ?
			 WHERE id = ? AND revision = ?`,
			write.snapshot.Revision, string(write.data), write.snapshot.CapturedAt.UnixNano(),
			write.snapshot.ID, write.expectedRevision,
		)
	}
	if err != nil {
		return fmt.Errorf("write process %q: %w", write.snapshot.ID, err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read affected rows for process %q: %w", write.snapshot.ID, err)
	}
	if changed == 1 {
		return nil
	}
	if changed != 0 {
		return fmt.Errorf("write process %q affected %d rows", write.snapshot.ID, changed)
	}

	actual := uint64(0)
	err = conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT revision FROM process_snapshots WHERE id = ?`, write.snapshot.ID,
	).Scan(&actual)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read conflicting revision for process %q: %w", write.snapshot.ID, err)
	}
	return &core.RevisionConflictError{
		ProcessID: write.snapshot.ID,
		Expected:  write.expectedRevision,
		Actual:    actual,
	}
}

// Load returns the snapshot for id, or an error wrapping
// [core.ErrSnapshotNotFound] when the id is unknown.
func (s *ProcessStore) Load(ctx context.Context, id string) (core.ProcessSnapshot, error) {
	var data string
	var revision uint64
	err := conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT revision, snapshot FROM process_snapshots WHERE id = ?`, id,
	).Scan(&revision, &data)
	if errors.Is(err, sql.ErrNoRows) {
		return core.ProcessSnapshot{}, fmt.Errorf("sqlite: load %q: %w", id, core.ErrSnapshotNotFound)
	}
	if err != nil {
		return core.ProcessSnapshot{}, fmt.Errorf("sqlite: load snapshot: %w", err)
	}
	var snap core.ProcessSnapshot
	if err := json.Unmarshal([]byte(data), &snap); err != nil {
		return core.ProcessSnapshot{}, fmt.Errorf("sqlite: parse snapshot %q: %w: %w", id, core.ErrInvalidSnapshot, err)
	}
	if snap.Revision != revision || revision == 0 {
		return core.ProcessSnapshot{}, fmt.Errorf("sqlite: snapshot %q revision mismatch: %w", id, core.ErrInvalidSnapshot)
	}
	return snap, nil
}

// Delete removes the snapshot for id. Idempotent — unknown id is not an
// error.
func (s *ProcessStore) Delete(ctx context.Context, id string) error {
	if _, err := conn(ctx, s.db).ExecContext(ctx,
		`DELETE FROM process_snapshots WHERE id = ?`, id,
	); err != nil {
		return fmt.Errorf("sqlite: delete snapshot: %w", err)
	}
	return nil
}

// DeleteTree removes root and every snapshot whose ParentID descends from it.
// It is idempotent and joins the caller's transaction through conn(ctx, s.db).
// Nested Runtime turns persist each process independently, so deleting only the
// root would leave child snapshots orphaned after parked cancel/run_lost.
func (s *ProcessStore) DeleteTree(ctx context.Context, rootID string) error {
	if rootID == "" {
		return nil
	}
	rows, err := conn(ctx, s.db).QueryContext(ctx,
		`SELECT id, snapshot FROM process_snapshots`)
	if err != nil {
		return fmt.Errorf("sqlite: list process tree: %w", err)
	}
	defer rows.Close()

	children := make(map[string][]string)
	for rows.Next() {
		var id, data string
		if err := rows.Scan(&id, &data); err != nil {
			return fmt.Errorf("sqlite: scan process tree: %w", err)
		}
		var snapshot core.ProcessSnapshot
		if err := json.Unmarshal([]byte(data), &snapshot); err != nil {
			return fmt.Errorf("sqlite: parse process tree snapshot %q: %w: %w", id, core.ErrInvalidSnapshot, err)
		}
		if snapshot.ID != id {
			return fmt.Errorf("sqlite: process tree snapshot id %q != row id %q: %w", snapshot.ID, id, core.ErrInvalidSnapshot)
		}
		if snapshot.ParentID != "" {
			children[snapshot.ParentID] = append(children[snapshot.ParentID], snapshot.ID)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("sqlite: iterate process tree: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("sqlite: close process tree rows: %w", err)
	}

	visited := make(map[string]struct{})
	var order []string
	var walk func(string)
	walk = func(id string) {
		if _, seen := visited[id]; seen {
			return
		}
		visited[id] = struct{}{}
		for _, childID := range children[id] {
			walk(childID)
		}
		order = append(order, id)
	}
	walk(rootID)
	for _, id := range order {
		if _, err := conn(ctx, s.db).ExecContext(ctx,
			`DELETE FROM process_snapshots WHERE id = ?`, id,
		); err != nil {
			return fmt.Errorf("sqlite: delete process tree snapshot %q: %w", id, err)
		}
	}
	return nil
}

// List returns every stored process id, most-recently-captured first.
func (s *ProcessStore) List(ctx context.Context) ([]string, error) {
	rows, err := conn(ctx, s.db).QueryContext(ctx,
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
