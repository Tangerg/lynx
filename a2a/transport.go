package a2a

import (
	"context"
	"fmt"
	"net/http"

	sdka2a "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/a2aproject/a2a-go/v2/a2aclient/agentcard"
)

// DialOptions configures a client connection opened by [Dial].
type DialOptions struct {
	// HTTPClient is the client used for both card resolution and RPC calls.
	// nil uses a default client.
	HTTPClient *http.Client
}

// httpClient returns the HTTP client to use, defaulting to the shared
// http.DefaultClient when none was supplied.
func (o DialOptions) httpClient() *http.Client {
	if o.HTTPClient != nil {
		return o.HTTPClient
	}
	return http.DefaultClient
}

// Dial resolves the remote AgentCard at cardURL and opens a client
// against it. The client negotiates a transport from the card's advertised
// interfaces; JSON-RPC and REST (both HTTP) are registered, matching the
// rest of the stack — a card that only advertises gRPC will fail to dial.
//
// The returned card is the resolved descriptor. Callers own the client and
// must Destroy it when done.
func Dial(ctx context.Context, cardURL string, opts DialOptions) (*a2aclient.Client, *sdka2a.AgentCard, error) {
	if cardURL == "" {
		return nil, nil, ErrEmptyCardURL
	}
	httpClient := opts.httpClient()

	resolver := agentcard.NewResolver(httpClient)
	card, err := resolver.Resolve(ctx, cardURL)
	if err != nil {
		return nil, nil, fmt.Errorf("a2a.Dial: resolve agent card at %q: %w", cardURL, err)
	}

	client, err := a2aclient.NewFromCard(ctx, card,
		a2aclient.WithJSONRPCTransport(httpClient),
		a2aclient.WithRESTTransport(httpClient),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("a2a.Dial: open client for %q: %w", card.Name, err)
	}
	return client, card, nil
}
