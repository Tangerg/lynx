// Package modelclient resolves per-(provider, model) chat and embedding clients
// from the runtime-mutable provider registry credentials, caching by the
// credential tuple so a credential mutation (new key or base URL) is picked up
// rather than serving a stale client. It is the driven adapter the runtime's
// per-run model selection, utility-model role, and semantic-index embedding role all
// resolve through.
package modelclient

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/llm"
	"github.com/Tangerg/lynx/chatclient"
)

// CredentialLookup is the model-client construction view of the provider
// registry: resolving a chat or embedding client needs only one provider's
// credentials, not list/configure capabilities.
type CredentialLookup interface {
	Get(ctx context.Context, id string) (provider.Provider, bool, error)
}

// ChatResolver resolves a per-Run [chatclient.Client] for an explicit model
// selection. The provider is taken as given by the selection and is never
// inferred from the model id; the resolver pulls that provider's credentials
// from the registry, then builds and caches the client.
type ChatResolver struct {
	providers CredentialLookup

	mu    sync.Mutex
	cache map[string]*chatclient.Client
}

// NewChatResolver returns a chat resolver over the provider credential lookup.
func NewChatResolver(providers CredentialLookup) *ChatResolver {
	return &ChatResolver{
		providers: providers,
		cache:     map[string]*chatclient.Client{},
	}
}

// ResolveChat returns the chat client for selection, building it from the
// provider's registry credentials. Errors when the provider isn't configured /
// enabled — the run then ends with a clear "set its API key first" error.
func (r *ChatResolver) ResolveChat(ctx context.Context, selection modelref.Selection) (*chatclient.Client, error) {
	if !selection.Configured() {
		return nil, errors.New("modelclient: explicit model selection is required")
	}
	providerID, model := selection.Provider(), selection.Model()
	entry, ok, err := r.providers.Get(ctx, providerID)
	if err != nil {
		return nil, err
	}
	if !ok || !entry.Enabled() {
		return nil, &run.Failure{
			Kind: run.FailureInvalidCredentials,
			Err:  fmt.Errorf("modelclient: provider %q is not configured (set its API key first)", providerID),
		}
	}

	// Key by everything that changes the built client, so a credential mutation
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
func (r *ChatResolver) ValidateChatModel(ctx context.Context, providerID, model string) error {
	selection, err := modelref.New(providerID, model)
	if err != nil {
		return err
	}
	_, err = r.ResolveChat(ctx, selection)
	return err
}
