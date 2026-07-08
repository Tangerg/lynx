package a2a

import (
	"context"
	"fmt"
	"net/http"

	sdka2a "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/a2aproject/a2a-go/v2/a2aclient/agentcard"
)

type dialOptions struct {
	// HTTPClient is the client used for both card resolution and RPC calls.
	// nil uses a default client.
	HTTPClient *http.Client
}

// httpClient returns the HTTP client to use, defaulting to the shared
// http.DefaultClient when none was supplied.
func (o dialOptions) httpClient() *http.Client {
	if o.HTTPClient != nil {
		return o.HTTPClient
	}
	return http.DefaultClient
}

func dial(ctx context.Context, cardURL string, opts dialOptions) (*a2aclient.Client, *sdka2a.AgentCard, error) {
	if cardURL == "" {
		return nil, nil, ErrEmptyCardURL
	}
	httpClient := opts.httpClient()

	resolver := agentcard.NewResolver(httpClient)
	card, err := resolver.Resolve(ctx, cardURL)
	if err != nil {
		return nil, nil, fmt.Errorf("a2a.dial: resolve agent card at %q: %w", cardURL, err)
	}
	if card == nil {
		return nil, nil, ErrNilCard
	}

	client, err := a2aclient.NewFromCard(ctx, card,
		a2aclient.WithJSONRPCTransport(httpClient),
		a2aclient.WithRESTTransport(httpClient),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("a2a.dial: open client for %q: %w", card.Name, err)
	}
	return client, card, nil
}
