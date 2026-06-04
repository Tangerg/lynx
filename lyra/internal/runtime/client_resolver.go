package runtime

import (
	"context"
	"fmt"
	"sync"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/lyra/internal/config"
	"github.com/Tangerg/lynx/lyra/internal/service/provider"
	"github.com/Tangerg/lynx/models/catalog"
)

// clientResolver resolves a per-turn [chat.Client] for a selected model — the
// [chatsvc.ClientResolver] the multi-provider runtime plugs into the chat
// service. It maps a model id to its provider (via the models catalog),
// pulls that provider's credentials from the registry, and builds (and
// caches) the client. Empty model resolves the runtime's default
// provider+model.
type clientResolver struct {
	providers   provider.Service
	defProvider config.Provider
	defModel    string

	mu    sync.Mutex
	cache map[string]*chat.Client
}

func newClientResolver(providers provider.Service, defProvider config.Provider, defModel string) *clientResolver {
	return &clientResolver{
		providers:   providers,
		defProvider: defProvider,
		defModel:    defModel,
		cache:       map[string]*chat.Client{},
	}
}

// ResolveClient returns the client for model, building it from the resolved
// provider's registry credentials. Errors when the model's provider isn't
// configured / enabled — the run then ends with an error the client can show
// (e.g. "fill in the key first").
func (r *clientResolver) ResolveClient(ctx context.Context, model string) (*chat.Client, error) {
	prov, modelID := r.resolve(model)

	entry, ok, err := r.providers.Get(ctx, string(prov))
	if err != nil {
		return nil, err
	}
	if !ok || !entry.Enabled() {
		return nil, fmt.Errorf("runtime: provider %q is not configured (set its API key first)", prov)
	}

	// Key by everything that changes the built client, so a providers.configure
	// (new key / base URL) is picked up rather than serving a stale client.
	key := string(prov) + "\x00" + modelID + "\x00" + entry.APIKey + "\x00" + entry.BaseURL
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.cache[key]; ok {
		return c, nil
	}
	client, _, err := config.BuildClient(config.ClientSpec{
		Provider: prov,
		Model:    modelID,
		APIKey:   entry.APIKey,
		BaseURL:  entry.BaseURL,
	})
	if err != nil {
		return nil, err
	}
	r.cache[key] = client
	return client, nil
}

// resolve maps a model id onto (provider, model): empty model → the runtime
// default; otherwise the first supported provider whose catalog lists the
// model, falling back to the default provider for an uncataloged model id
// (best effort — the adapter passes it through).
func (r *clientResolver) resolve(model string) (config.Provider, string) {
	if model == "" {
		return r.defProvider, r.defModel
	}
	for _, p := range config.SupportedProviders() {
		if _, ok := catalog.Lookup(string(p), model); ok {
			return p, model
		}
	}
	return r.defProvider, model
}
