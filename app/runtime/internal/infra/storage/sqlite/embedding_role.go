package sqlite

import (
	"context"
	"database/sql"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
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
	return &EmbeddingRoleStore{store: newRoleStore(db, "embedding_role", "embedding role")}
}

// LoadEmbeddingRole returns the stored role, or its zero value when unset (no
// row yet) — the index feature is then off until one is configured.
func (s *EmbeddingRoleStore) LoadEmbeddingRole(ctx context.Context) (modelref.Selection, error) {
	return s.store.load(ctx)
}

// SaveEmbeddingRole upserts the single embedding-role row. A zero role clears
// it (turns the index feature off).
func (s *EmbeddingRoleStore) SaveEmbeddingRole(ctx context.Context, role modelref.Selection) error {
	return s.store.save(ctx, role)
}
