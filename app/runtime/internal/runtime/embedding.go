package runtime

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/Tangerg/lynx/core/model/embedding"

	codebaseindexadapter "github.com/Tangerg/lynx/app/runtime/internal/adapter/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelrole"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/llm"
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
	resolver *embeddingResolver
	index    codebaseindex.Index
}

func buildEmbeddingEnvironment(ctx context.Context, roleStore embeddingRoleLoader, indexStore codebaseindex.Store, providers providerCredentialLookup) (embeddingEnvironment, error) {
	resolver := newEmbeddingResolver(providers)
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
		return resolver.resolve(ctx, role.ProviderID(), role.Model())
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
		if _, err := r.embeddings.resolve(ctx, role.ProviderID(), role.Model()); err != nil {
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

// embeddingResolver builds + caches embedding clients from provider-registry
// credentials, keyed by everything that changes the built client (so a
// providers.configure is picked up). Mirrors [clientResolver].
type embeddingResolver struct {
	providers providerCredentialLookup
	mu        sync.Mutex
	cache     map[string]codebaseindex.Embedder
}

func newEmbeddingResolver(providers providerCredentialLookup) *embeddingResolver {
	return &embeddingResolver{providers: providers, cache: map[string]codebaseindex.Embedder{}}
}

func (r *embeddingResolver) resolve(ctx context.Context, providerID, model string) (codebaseindex.Embedder, error) {
	entry, ok, err := r.providers.Get(ctx, providerID)
	if err != nil {
		return nil, err
	}
	if !ok || !entry.Enabled() {
		return nil, fmt.Errorf("runtime: provider %q is not configured (set its API key first)", providerID)
	}
	key := providerID + "\x00" + model + "\x00" + entry.APIKey + "\x00" + entry.BaseURL
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.cache[key]; ok {
		return e, nil
	}
	m, err := llm.BuildEmbeddingModel(llm.ClientSpec{
		Provider: llm.Provider(providerID),
		Model:    model,
		APIKey:   entry.APIKey,
		BaseURL:  entry.BaseURL,
	})
	if err != nil {
		return nil, err
	}
	client, err := embedding.NewClient(m)
	if err != nil {
		return nil, err
	}
	e := &embedder{id: providerID + ":" + model, client: client}
	r.cache[key] = e
	return e, nil
}

// embedder adapts an embedding.Client to [codebaseindex.Embedder], converting
// the float64 vectors to the float32 the index stores.
type embedder struct {
	id     string
	client *embedding.Client
}

func (e *embedder) ID() string { return e.id }

func (e *embedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	vecs, _, err := e.client.EmbedWithTexts(texts).Call().Embeddings(ctx)
	if err != nil {
		return nil, err
	}
	out := make([][]float32, len(vecs))
	for i, v := range vecs {
		f := make([]float32, len(v))
		for j, x := range v {
			f[j] = float32(x)
		}
		out[i] = f
	}
	return out, nil
}
