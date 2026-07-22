package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// SandboxSnapshotStore persists immutable, content-addressed workspace tar
// archives. Live sandbox clients and process state are deliberately absent.
//
// It is the durable backing for the sandbox package's isolated-copy Workspace,
// wired via internal/adapter/isolation: an isolated session's final working-copy
// state is snapshotted here when the session is discarded.
type SandboxSnapshotStore struct {
	db *sql.DB
}

// NewSandboxSnapshotStore binds a database opened by Open.
func NewSandboxSnapshotStore(db *sql.DB) *SandboxSnapshotStore {
	return &SandboxSnapshotStore{db: db}
}

// SaveSandboxSnapshot inserts an immutable archive or verifies an idempotent
// retry against the bytes already stored under id.
func (s *SandboxSnapshotStore) SaveSandboxSnapshot(ctx context.Context, id string, archive []byte) error {
	if id == "" {
		return errors.New("sqlite: sandbox snapshot id is required")
	}
	if len(archive) == 0 {
		return errors.New("sqlite: sandbox snapshot archive is empty")
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO sandbox_snapshots(id, archive, size) VALUES (?, ?, ?)`,
		id, archive, len(archive)); err != nil {
		return fmt.Errorf("sqlite: save sandbox snapshot %q: %w", id, err)
	}
	var stored []byte
	if err := s.db.QueryRowContext(ctx,
		`SELECT archive FROM sandbox_snapshots WHERE id = ?`, id).Scan(&stored); err != nil {
		return fmt.Errorf("sqlite: verify sandbox snapshot %q: %w", id, err)
	}
	if !bytes.Equal(stored, archive) {
		return fmt.Errorf("sqlite: sandbox snapshot %q conflicts with different content", id)
	}
	return nil
}

// LoadSandboxSnapshot returns id's archive and found=false for an unknown
// reference.
func (s *SandboxSnapshotStore) LoadSandboxSnapshot(ctx context.Context, id string) ([]byte, bool, error) {
	var archive []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT archive FROM sandbox_snapshots WHERE id = ?`, id).Scan(&archive)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("sqlite: load sandbox snapshot %q: %w", id, err)
	}
	return archive, true, nil
}
