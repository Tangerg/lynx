package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

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

// NewProcessStore binds the SQLite process-store to a database opened via
// [Open].
func NewProcessStore(db *sql.DB) *ProcessStore {
	return &ProcessStore{db: db}
}

// Apply commits one validated process-snapshot mutation in one transaction.
func (s *ProcessStore) Apply(ctx context.Context, mutation core.SnapshotMutation) error {
	if err := mutation.Validate(); err != nil {
		return fmt.Errorf("sqlite: process snapshot mutation: %w", err)
	}
	prepared, err := prepareSnapshotWrites(mutation.Writes)
	if err != nil {
		return err
	}
	if len(prepared) == 0 && len(mutation.DeleteTrees) == 0 {
		return nil
	}

	err = RunInTx(ctx, s.db, func(ctx context.Context) error {
		var stored map[string]core.ProcessSnapshot
		if len(mutation.DeleteTrees) > 0 {
			stored, err = s.loadStoredSnapshots(ctx)
			if err != nil {
				return err
			}
		}
		deleteSet := processTreeDeleteSet(stored, mutation.DeleteTrees)
		if err := validateProcessMutationLineage(stored, mutation.Writes, deleteSet); err != nil {
			return err
		}
		for index := range prepared {
			if err := s.savePreparedSnapshot(ctx, prepared[index]); err != nil {
				return fmt.Errorf("sqlite: process snapshot mutation write[%d]: %w", index, err)
			}
		}
		deleteIDs := make([]string, 0, len(deleteSet))
		for id := range deleteSet {
			deleteIDs = append(deleteIDs, id)
		}
		slices.Sort(deleteIDs)
		for index, id := range deleteIDs {
			if _, err := conn(ctx, s.db).ExecContext(ctx,
				`DELETE FROM process_snapshots WHERE id = ?`, id,
			); err != nil {
				return fmt.Errorf("sqlite: process snapshot mutation tree delete[%d] %q: %w", index, id, err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func (s *ProcessStore) loadStoredSnapshots(ctx context.Context) (map[string]core.ProcessSnapshot, error) {
	rows, err := conn(ctx, s.db).QueryContext(ctx, `SELECT id, snapshot FROM process_snapshots`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: load process snapshot graph: %w", err)
	}
	defer rows.Close()
	stored := make(map[string]core.ProcessSnapshot)
	for rows.Next() {
		var id, data string
		if err := rows.Scan(&id, &data); err != nil {
			return nil, fmt.Errorf("sqlite: scan process snapshot graph: %w", err)
		}
		var snapshot core.ProcessSnapshot
		if err := json.Unmarshal([]byte(data), &snapshot); err != nil {
			return nil, fmt.Errorf("sqlite: parse process tree snapshot %q: %w: %w", id, core.ErrInvalidSnapshot, err)
		}
		if snapshot.ID != id {
			return nil, fmt.Errorf("sqlite: process tree snapshot id %q != row id %q: %w", snapshot.ID, id, core.ErrInvalidSnapshot)
		}
		stored[id] = snapshot
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterate process snapshot graph: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("sqlite: close process snapshot graph: %w", err)
	}
	return stored, nil
}

func processTreeDeleteSet(stored map[string]core.ProcessSnapshot, roots []string) map[string]struct{} {
	if len(roots) == 0 {
		return nil
	}
	children := make(map[string][]string)
	for id, snapshot := range stored {
		if snapshot.ParentID != "" {
			children[snapshot.ParentID] = append(children[snapshot.ParentID], id)
		}
	}
	deleted := make(map[string]struct{})
	var walk func(string)
	walk = func(id string) {
		if _, visited := deleted[id]; visited {
			return
		}
		deleted[id] = struct{}{}
		for _, childID := range children[id] {
			walk(childID)
		}
	}
	for _, root := range roots {
		walk(root)
	}
	return deleted
}

func validateProcessMutationLineage(
	stored map[string]core.ProcessSnapshot,
	writes []core.ProcessSnapshot,
	deleted map[string]struct{},
) error {
	if len(deleted) == 0 {
		return nil
	}
	pending := make(map[string]core.ProcessSnapshot, len(writes))
	for _, snapshot := range writes {
		pending[snapshot.ID] = snapshot
	}
	for _, snapshot := range writes {
		if _, removed := deleted[snapshot.ID]; removed {
			return fmt.Errorf("sqlite: process snapshot mutation: %w: write process %q belongs to a deleted tree", core.ErrInvalidSnapshot, snapshot.ID)
		}
		visited := map[string]struct{}{snapshot.ID: {}}
		for parentID := snapshot.ParentID; parentID != ""; {
			if _, removed := deleted[parentID]; removed {
				return fmt.Errorf("sqlite: process snapshot mutation: %w: write process %q descends from deleted process %q", core.ErrInvalidSnapshot, snapshot.ID, parentID)
			}
			if _, duplicate := visited[parentID]; duplicate {
				return fmt.Errorf("sqlite: process snapshot mutation: %w: write process %q has cyclic lineage", core.ErrInvalidSnapshot, snapshot.ID)
			}
			visited[parentID] = struct{}{}
			parent, ok := pending[parentID]
			if !ok {
				parent, ok = stored[parentID]
			}
			if !ok {
				break
			}
			parentID = parent.ParentID
		}
	}
	return nil
}

type preparedSnapshotWrite struct {
	snapshot         core.ProcessSnapshot
	expectedRevision uint64
	data             []byte
}

func prepareSnapshotWrites(snapshots []core.ProcessSnapshot) ([]preparedSnapshotWrite, error) {
	prepared := make([]preparedSnapshotWrite, len(snapshots))
	for index, snapshot := range snapshots {
		candidate := snapshot
		candidate.Revision++
		data, err := json.Marshal(candidate)
		if err != nil {
			return nil, fmt.Errorf("sqlite: process snapshot mutation write[%d]: marshal: %w", index, err)
		}
		prepared[index] = preparedSnapshotWrite{
			snapshot:         candidate,
			expectedRevision: snapshot.Revision,
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
