package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
)

// WorkspaceMutationStore is the recoverable operation log for file rollbacks
// (§8.5), backed by the pending_workspace_mutations table. Unlike the write-set
// stores, its writes deliberately do NOT join an ambient transaction (they use
// the *sql.DB directly, never conn(ctx)): the intent must commit on its own
// before the working tree is touched, and the completion on its own after the
// requested effects commit. The row protects a non-atomic multi-path Git reset
// and, when requested, the separate SQLite history transaction.
//
// Safe for concurrent use; the *sql.DB serializes writes (MaxOpenConns 1, see
// [Open]).
type WorkspaceMutationStore struct {
	db *sql.DB
}

// NewWorkspaceMutationStore wires a database opened via [Open] to the
// operation-log surface.
func NewWorkspaceMutationStore(db *sql.DB) *WorkspaceMutationStore {
	return &WorkspaceMutationStore{db: db}
}

// Record logs a rollback's intent before the working tree is touched.
// INSERT OR REPLACE is idempotent against a leftover row for the same session
// (the mutation slot admits one in-flight rollback per session, so this is
// effectively an insert). created_at is stamped by the DB default.
func (s *WorkspaceMutationStore) Record(ctx context.Context, m execution.WorkspaceMutation) error {
	if s == nil {
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO pending_workspace_mutations(session_id, cwd, to_run_id, restore_history) VALUES (?, ?, ?, ?)`,
		m.SessionID, m.Cwd, m.ToRunID, m.RestoreHistory)
	if err != nil {
		return fmt.Errorf("sqlite: record workspace mutation: %w", err)
	}
	return nil
}

// Complete clears a session's logged intent once the file restore and, when
// requested, durable truncation have committed. Idempotent: deleting an absent
// row is not an error, so re-completion is a no-op.
func (s *WorkspaceMutationStore) Complete(ctx context.Context, sessionID string) error {
	if s == nil {
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM pending_workspace_mutations WHERE session_id = ?`, sessionID)
	if err != nil {
		return fmt.Errorf("sqlite: complete workspace mutation: %w", err)
	}
	return nil
}

// ListPending returns every rollback a crash left unfinished, oldest first, for
// boot recovery to re-drive.
func (s *WorkspaceMutationStore) ListPending(ctx context.Context) ([]execution.WorkspaceMutation, error) {
	if s == nil {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT session_id, cwd, to_run_id, restore_history FROM pending_workspace_mutations ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list workspace mutations: %w", err)
	}
	defer rows.Close()

	var out []execution.WorkspaceMutation
	for rows.Next() {
		var m execution.WorkspaceMutation
		if err := rows.Scan(&m.SessionID, &m.Cwd, &m.ToRunID, &m.RestoreHistory); err != nil {
			return nil, fmt.Errorf("sqlite: scan workspace mutation: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterate workspace mutations: %w", err)
	}
	return out, nil
}
