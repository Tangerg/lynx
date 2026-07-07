package runtime

import (
	"context"
	"fmt"
	"sync"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/llm"
	"github.com/Tangerg/lynx/core/model/chat"
)

// clientResolver resolves a per-turn [chat.Client] for an explicit
// (provider, model) — it satisfies the chat service's own (unexported)
// client-resolver seam the multi-provider runtime plugs in. The provider is taken as given (the wire
// carries it; it's never inferred from the model id); the resolver pulls that
// provider's credentials from the registry, then builds and caches the client.
type clientResolver struct {
	providers provider.Registry

	mu    sync.Mutex
	cache map[string]*chat.Client
}

func newClientResolver(providers provider.Registry) *clientResolver {
	return &clientResolver{
		providers: providers,
		cache:     map[string]*chat.Client{},
	}
}

// ResolveClient returns the client for (provider, model), building it from the
// provider's registry credentials. Errors when the provider isn't configured /
// enabled — the run then ends with a clear "set its API key first" error.
func (r *clientResolver) ResolveClient(ctx context.Context, providerID, model string) (*chat.Client, error) {
	entry, ok, err := r.providers.Get(ctx, providerID)
	if err != nil {
		return nil, err
	}
	if !ok || !entry.Enabled() {
		return nil, fmt.Errorf("runtime: provider %q is not configured (set its API key first)", providerID)
	}

	// Key by everything that changes the built client, so a providers.configure
	// (new key / base URL) is picked up rather than serving a stale client.
	key := providerID + "\x00" + model + "\x00" + entry.APIKey + "\x00" + entry.BaseURL
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.cache[key]; ok {
		return c, nil
	}
	client, err := llm.BuildClient(llm.ClientSpec{
		Provider: llm.Provider(providerID),
		Model:    model,
		APIKey:   entry.APIKey,
		BaseURL:  entry.BaseURL,
	})
	if err != nil {
		return nil, err
	}
	r.cache[key] = client
	return client, nil
}
