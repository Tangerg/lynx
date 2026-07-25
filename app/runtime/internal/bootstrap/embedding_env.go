package bootstrap

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
)

// embeddingRoleLoader is the boot-time load view of the embedding-role store
// (persistence save belongs to the capabilities coordinator's SetEmbeddingRole).
type embeddingRoleLoader interface {
	LoadEmbeddingRole(ctx context.Context) (modelref.Selection, error)
}

// loadEmbeddingRole reads the persisted startup assignment. Runtime mutation
// and embedding resolution belong to their owning application and adapter types.
func loadEmbeddingRole(ctx context.Context, roleStore embeddingRoleLoader) (modelref.Selection, error) {
	var role modelref.Selection
	if roleStore != nil {
		loaded, err := roleStore.LoadEmbeddingRole(ctx)
		if err != nil {
			return modelref.Selection{}, fmt.Errorf("bootstrap: load embedding role: %w", err)
		}
		role = loaded
	}
	return role, nil
}
