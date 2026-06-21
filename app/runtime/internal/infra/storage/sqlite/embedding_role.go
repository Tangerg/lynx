package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// EmbeddingRoleStore persists the global embedding-model role — the (provider,
// model) the @codebase semantic index embeds with — as a single row. The DB
// must have been opened via [Open] so the embedding_role table exists. Mirrors
// [UtilityRoleStore]; the credential for the named provider comes from the
// provider registry.
type EmbeddingRoleStore struct {
	db *sql.DB
}

// NewEmbeddingRoleStore wires the given *sql.DB to the embedding-role surface.
func NewEmbeddingRoleStore(db *sql.DB) *EmbeddingRoleStore {
	return &EmbeddingRoleStore{db: db}
}

// LoadEmbeddingRole returns the stored (provider, model); both empty when unset
// (no row yet) — the index feature is then off until one is configured.
func (s *EmbeddingRoleStore) LoadEmbeddingRole(ctx context.Context) (provider, model string, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT provider, model FROM embedding_role WHERE id = 1`).
		Scan(&provider, &model)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("sqlite: load embedding role: %w", err)
	}
	return provider, model, nil
}

// SaveEmbeddingRole upserts the single embedding-role row. An empty model clears
// the role (turns the index feature off).
func (s *EmbeddingRoleStore) SaveEmbeddingRole(ctx context.Context, provider, model string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO embedding_role (id, provider, model) VALUES (1, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET provider = excluded.provider, model = excluded.model`,
		provider, model)
	if err != nil {
		return fmt.Errorf("sqlite: save embedding role: %w", err)
	}
	return nil
}
