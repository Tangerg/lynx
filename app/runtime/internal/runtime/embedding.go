package runtime

import (
	"context"
	"fmt"
	"sync/atomic"

	codebaseindexadapter "github.com/Tangerg/lynx/app/runtime/internal/adapter/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/modelclient"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelrole"
)

// EmbeddingRoleStore persists the embedding-model role across restarts. Defined
// here (consumer side); the composition root injects the sqlite-backed impl. A
// nil store disables persistence — the role stays whatever was last set in-proc.
type EmbeddingRoleStore interface {
	embeddingRoleLoader
	embeddingRoleSaver
}

type embeddingRoleLoader interface {
	LoadEmbeddingRole(ctx context.Context) (provider, model string, err error)
}

type embeddingRoleSaver interface {
	SaveEmbeddingRole(ctx context.Context, provider, model string) error
}

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
			return embeddingEnvironment{}, fmt.Errorf("runtime: load embedding role: %w", err)
		}
		role, err = modelrole.New(p, m)
		if err != nil {
			return embeddingEnvironment{}, fmt.Errorf("runtime: load embedding role: %w", err)
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

// EmbeddingRole returns the live embedding role; both empty when unset. Backs
// models.getEmbeddingRole.
func (r *Runtime) EmbeddingRole() (providerID, model string) {
	role := r.embeddingCell.Load()
	if role == nil {
		return "", ""
	}
	return role.ProviderID(), role.Model()
}

// SetEmbeddingRole repoints the @codebase index at (provider, model), persists
// it, and swaps the live cell. An empty model clears the role (turns the index
// off). A non-empty model is validated by building its embedding client, so an
// unbuildable role fails here rather than at the next search. The caller is
// expected to have already rejected a non-embedding-capable or unconfigured
// provider (the delivery layer does, as invalid_params), so a failure here is an
// internal one. Backs models.setEmbeddingRole.
func (r *Runtime) SetEmbeddingRole(ctx context.Context, providerID, model string) error {
	role, err := modelrole.New(providerID, model)
	if err != nil {
		return err
	}
	if role.Configured() {
		if _, err := r.embeddings.Resolve(ctx, role.ProviderID(), role.Model()); err != nil {
			return fmt.Errorf("runtime: build embedding model %q on %q: %w", role.Model(), role.ProviderID(), err)
		}
	}
	if r.embeddingStore != nil {
		if err := r.embeddingStore.SaveEmbeddingRole(ctx, role.ProviderID(), role.Model()); err != nil {
			return fmt.Errorf("runtime: persist embedding role: %w", err)
		}
	}
	r.embeddingCell.Store(&role)
	return nil
}
