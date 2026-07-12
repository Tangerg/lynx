package bootstrap

import (
	"context"
	"fmt"
	"sync/atomic"

	codebaseindexadapter "github.com/Tangerg/lynx/app/runtime/internal/adapter/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/modelclient"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelrole"
)

// embeddingRoleLoader is the boot-time load view of the embedding-role store
// (persistence save belongs to the capabilities coordinator's SetEmbeddingRole).
type embeddingRoleLoader interface {
	LoadEmbeddingRole(ctx context.Context) (provider, model string, err error)
}

// embeddingEnvironment is the boot-time @codebase wiring: the live embedding
// role cell (repointed by the capabilities coordinator's SetEmbeddingRole), the embedding
// client resolver, and the index built over them (nil when no index store).
type embeddingEnvironment struct {
	cell     *atomic.Pointer[modelrole.Role]
	resolver *modelclient.EmbeddingResolver
	index    codebaseindex.Index
}

func buildEmbeddingEnvironment(ctx context.Context, roleStore embeddingRoleLoader, indexStore codebaseindex.Store, providers modelclient.CredentialLookup) (embeddingEnvironment, error) {
	resolver := modelclient.NewEmbeddingResolver(providers)
	cell := &atomic.Pointer[modelrole.Role]{}
	var role modelrole.Role
	if roleStore != nil {
		p, m, err := roleStore.LoadEmbeddingRole(ctx)
		if err != nil {
			return embeddingEnvironment{}, fmt.Errorf("bootstrap: load embedding role: %w", err)
		}
		role, err = modelrole.New(p, m)
		if err != nil {
			return embeddingEnvironment{}, fmt.Errorf("bootstrap: load embedding role: %w", err)
		}
	}
	cell.Store(&role)
	resolveEmbedder := func(ctx context.Context) (codebaseindex.Embedder, error) {
		role := cell.Load()
		if role == nil || !role.Configured() {
			return nil, codebaseindex.ErrNoEmbeddingModel
		}
		return resolver.Resolve(ctx, role.ProviderID(), role.Model())
	}
	var index codebaseindex.Index
	if indexStore != nil {
		index = codebaseindex.New(indexStore, resolveEmbedder, codebaseindexadapter.Source{})
	}
	return embeddingEnvironment{cell: cell, resolver: resolver, index: index}, nil
}
