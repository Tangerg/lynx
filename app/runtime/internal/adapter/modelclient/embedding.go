package modelclient

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/Tangerg/lynx/embeddingclient"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/llm"
)

// EmbeddingResolver builds + caches embedding clients from provider-registry
// credentials, keyed by everything that changes the built client (so a
// credential mutation is picked up). Mirrors [ClientResolver] for semantic-index
// embedding role.
type EmbeddingResolver struct {
	providers CredentialLookup
	mu        sync.Mutex
	cache     map[string]codebaseindex.Embedder
}

// NewEmbeddingResolver returns a resolver over the provider credential lookup.
func NewEmbeddingResolver(providers CredentialLookup) *EmbeddingResolver {
	return &EmbeddingResolver{providers: providers, cache: map[string]codebaseindex.Embedder{}}
}

// Resolve builds (or returns a cached) embedder for selection.
func (r *EmbeddingResolver) Resolve(ctx context.Context, selection modelref.Selection) (codebaseindex.Embedder, error) {
	if !selection.Configured() {
		return nil, errors.New("modelclient: explicit model selection is required")
	}
	providerID, model := selection.Provider(), selection.Model()
	entry, ok, err := r.providers.Get(ctx, providerID)
	if err != nil {
		return nil, err
	}
	if !ok || !entry.Enabled() {
		return nil, fmt.Errorf("modelclient: provider %q is not configured (set its API key first)", providerID)
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
	client, err := embeddingclient.New(m)
	if err != nil {
		return nil, err
	}
	e := &embedder{id: providerID + ":" + model, client: client}
	r.cache[key] = e
	return e, nil
}

// ValidateEmbeddingModel implements the application role-validation port while
// keeping the usable embedder inside the adapter that owns it.
func (r *EmbeddingResolver) ValidateEmbeddingModel(ctx context.Context, providerID, model string) error {
	selection, err := modelref.New(providerID, model)
	if err != nil {
		return err
	}
	_, err = r.Resolve(ctx, selection)
	return err
}

// embedder adapts an embeddingclient.Client to [codebaseindex.Embedder], converting
// the float64 vectors to the float32 the index stores.
type embedder struct {
	id     string
	client *embeddingclient.Client
}

func (e *embedder) ID() string { return e.id }

func (e *embedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	vecs, err := e.client.EmbedTexts(ctx, texts)
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
