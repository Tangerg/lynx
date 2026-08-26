package sqlite

import (
	"context"
	"database/sql"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
)

// EmbeddingRoleStore persists the optional agent-memory embedding-model role as
// a single row. The DB
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
func (e *EmbeddingRoleStore) LoadEmbeddingRole(ctx context.Context) (modelref.Selection, error) {
	return e.store.load(ctx)
}

// SaveEmbeddingRole upserts the single embedding-role row. A zero role clears
// it (turns the index feature off).
func (e *EmbeddingRoleStore) SaveEmbeddingRole(ctx context.Context, role modelref.Selection) error {
	return e.store.save(ctx, role)
}
