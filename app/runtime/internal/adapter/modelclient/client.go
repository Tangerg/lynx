// Package modelclient resolves per-(provider, model) chat and embedding clients
// from the runtime-mutable provider registry credentials, caching by the
// credential tuple so a providers.configure (new key / base URL) is picked up
// rather than serving a stale client. It is the driven adapter the runtime's
// per-run model selection, utility-model role, and @codebase embedding role all
// resolve through.
package modelclient

import (
	"context"
	"fmt"
	"sync"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/llm"
	"github.com/Tangerg/lynx/chatclient"
)

// CredentialLookup is the model-client construction view of the provider
// registry: resolving a chat or embedding client needs only one provider's
// credentials, not list/configure capabilities.
type CredentialLookup interface {
	Get(ctx context.Context, id string) (provider.Provider, bool, error)
}

// ClientResolver resolves a per-turn [chatclient.Client] for an explicit
// (provider, model). The provider is taken as given (the wire carries it; it is
// never inferred from the model id); the resolver pulls that provider's
// credentials from the registry, then builds and caches the client.
type ClientResolver struct {
	providers CredentialLookup

	mu    sync.Mutex
	cache map[string]*chatclient.Client
}

// NewClientResolver returns a resolver over the provider credential lookup.
func NewClientResolver(providers CredentialLookup) *ClientResolver {
	return &ClientResolver{
		providers: providers,
		cache:     map[string]*chatclient.Client{},
	}
}

// ResolveClient returns the client for (provider, model), building it from the
// provider's registry credentials. Errors when the provider isn't configured /
// enabled — the run then ends with a clear "set its API key first" error.
func (r *ClientResolver) ResolveClient(ctx context.Context, providerID, model string) (*chatclient.Client, error) {
	entry, ok, err := r.providers.Get(ctx, providerID)
	if err != nil {
		return nil, err
	}
	if !ok || !entry.Enabled() {
		return nil, fmt.Errorf("modelclient: provider %q is not configured (set its API key first)", providerID)
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

// ValidateChatModel implements the application model-role validation port
// without leaking the concrete chat client into the use-case layer.
func (r *ClientResolver) ValidateChatModel(ctx context.Context, providerID, model string) error {
	_, err := r.ResolveClient(ctx, providerID, model)
	return err
}
