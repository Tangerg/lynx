package sqlite

import (
	"context"
	"database/sql"
)

// EmbeddingRoleStore persists the global embedding-model role — the (provider,
// model) the @codebase semantic index embeds with — as a single row. The DB
// must have been opened via [Open] so the embedding_role table exists. Mirrors
// [UtilityRoleStore]; the credential for the named provider comes from the
// provider registry.
type EmbeddingRoleStore struct {
	store *roleStore
}

// NewEmbeddingRoleStore wires the given *sql.DB to the embedding-role surface.
func NewEmbeddingRoleStore(db *sql.DB) *EmbeddingRoleStore {
	return &EmbeddingRoleStore{store: newRoleStore(db, "embedding_role", "embedding role", "embedding role")}
}

// LoadEmbeddingRole returns the stored (provider, model); both empty when unset
// (no row yet) — the index feature is then off until one is configured.
func (s *EmbeddingRoleStore) LoadEmbeddingRole(ctx context.Context) (provider, model string, err error) {
	return s.store.load(ctx)
}

// SaveEmbeddingRole upserts the single embedding-role row. An empty model
// clears the role (turns the index feature off).
func (s *EmbeddingRoleStore) SaveEmbeddingRole(ctx context.Context, provider, model string) error {
	return s.store.save(ctx, provider, model)
}
