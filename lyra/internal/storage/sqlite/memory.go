package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/lyra/internal/service/memory"
)

// MemoryService implements memory.Service against a SQLite database.
// Unlike FileMemoryService (which stores per-scope LYRA.md files the user
// can edit directly with their editor), the SQLite variant centralises
// content in one DB — trading direct-edit ergonomics for a single
// source of truth that's easy to back up / replicate.
//
// One row per scope, keyed on the Scope integer. Update is UPSERT-style
// (INSERT … ON CONFLICT DO UPDATE) so first-time writes and overwrites
// share the same code path.
type MemoryService struct {
	db *sql.DB
}

// NewMemoryService binds the SQLite memory implementation to db. db
// must have been opened via [Open] so the migration ran.
func NewMemoryService(db *sql.DB) *MemoryService {
	return &MemoryService{db: db}
}

// Get returns the stored content for scope. Missing rows return ""
// (matches FileMemoryService — a non-existent LYRA.md is not an error).
func (m *MemoryService) Get(ctx context.Context, scope memory.Scope) (string, error) {
	var content string
	err := m.db.QueryRowContext(ctx,
		`SELECT content FROM memory WHERE scope = ?`, int(scope),
	).Scan(&content)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("sqlite: get memory: %w", err)
	}
	return content, nil
}

// Update writes content for scope. UPSERT semantics: first write inserts,
// subsequent writes overwrite. captured_at is refreshed on every write.
func (m *MemoryService) Update(ctx context.Context, scope memory.Scope, content string) error {
	_, err := m.db.ExecContext(ctx,
		`INSERT INTO memory(scope, content, captured_at) VALUES (?, ?, ?)
		 ON CONFLICT(scope) DO UPDATE SET
		   content     = excluded.content,
		   captured_at = excluded.captured_at`,
		int(scope), content, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("sqlite: update memory: %w", err)
	}
	return nil
}

// List returns one Entry per scope that has content. Empty rows are
// skipped — UIs shouldn't render placeholder entries.
func (m *MemoryService) List(ctx context.Context) ([]memory.Entry, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT scope, content, captured_at FROM memory
		 WHERE content != ''
		 ORDER BY scope`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list memory: %w", err)
	}
	defer rows.Close()

	out := make([]memory.Entry, 0)
	for rows.Next() {
		var (
			scope int
			entry memory.Entry
		)
		if err := rows.Scan(&scope, &entry.Content, &entry.CapturedAt); err != nil {
			return nil, fmt.Errorf("sqlite: scan memory: %w", err)
		}
		entry.Scope = memory.Scope(scope)
		out = append(out, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list memory: %w", err)
	}
	return out, nil
}
