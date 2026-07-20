package bootstrap

import (
	"context"
	"fmt"
	"sync/atomic"

	codebaseindexadapter "github.com/Tangerg/lynx/app/runtime/internal/adapter/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/modelclient"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelrole"
)

// memoryEmbedder bridges the @codebase embedder resolver to the agent-memory
// Embedder surface (an identical method set), so both features embed through the
// one live embedding role. nil in → nil out (keyword-only memory search).
func memoryEmbedder(resolve func(context.Context) (codebaseindex.Embedder, error)) func(context.Context) (agentmemory.Embedder, error) {
	if resolve == nil {
		return nil
	}
	return func(ctx context.Context) (agentmemory.Embedder, error) {
		embedder, err := resolve(ctx)
		if err != nil {
			return nil, err
		}
		return embedder, nil
	}
}

// embeddingRoleLoader is the boot-time load view of the embedding-role store
// (persistence save belongs to the capabilities coordinator's SetEmbeddingRole).
type embeddingRoleLoader interface {
	LoadEmbeddingRole(ctx context.Context) (provider, model string, err error)
}

// embeddingEnvironment is the boot-time @codebase wiring: the live embedding
// role cell (repointed by the capabilities coordinator's SetEmbeddingRole), the embedding
// client resolver, the index built over them (nil when no index store), and the
// live embedder resolver both the index and agent-memory search embed queries
// through.
type embeddingEnvironment struct {
	cell            *atomic.Pointer[modelrole.Role]
	resolver        *modelclient.EmbeddingResolver
	index           codebaseindex.Index
	resolveEmbedder func(context.Context) (codebaseindex.Embedder, error)
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
	return embeddingEnvironment{cell: cell, resolver: resolver, index: index, resolveEmbedder: resolveEmbedder}, nil
}
