package bootstrap

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelrole"
)

// embeddingRoleLoader is the boot-time load view of the embedding-role store
// (persistence save belongs to the capabilities coordinator's SetEmbeddingRole).
type embeddingRoleLoader interface {
	LoadEmbeddingRole(ctx context.Context) (provider, model string, err error)
}

// loadEmbeddingRole reads the persisted startup assignment. Runtime mutation
// and embedding resolution belong to their owning application and adapter types.
func loadEmbeddingRole(ctx context.Context, roleStore embeddingRoleLoader) (modelrole.Role, error) {
	var role modelrole.Role
	if roleStore != nil {
		p, m, err := roleStore.LoadEmbeddingRole(ctx)
		if err != nil {
			return modelrole.Role{}, fmt.Errorf("bootstrap: load embedding role: %w", err)
		}
		role, err = modelrole.New(p, m)
		if err != nil {
			return modelrole.Role{}, fmt.Errorf("bootstrap: load embedding role: %w", err)
		}
	}
	return role, nil
}
