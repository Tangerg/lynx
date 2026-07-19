package a2a

import (
	"context"
	"fmt"
	"net/http"
	"time"

	sdka2a "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/a2aproject/a2a-go/v2/a2aclient/agentcard"
)

type dialOptions struct {
	// HTTPClient is the client used for both card resolution and RPC calls.
	// nil uses a default client.
	HTTPClient *http.Client

	CardTimeout       time.Duration
	AllowedRPCOrigins []string
}

// httpClient returns the HTTP client to use, defaulting to the shared
// http.DefaultClient when none was supplied.
func (o dialOptions) httpClient() *http.Client {
	if o.HTTPClient != nil {
		return o.HTTPClient
	}
	return http.DefaultClient
}

const defaultCardTimeout = 30 * time.Second

func dial(ctx context.Context, cardURL string, opts dialOptions) (*a2aclient.Client, *sdka2a.AgentCard, error) {
	if cardURL == "" {
		return nil, nil, ErrEmptyCardURL
	}
	if opts.CardTimeout < 0 {
		return nil, nil, ErrInvalidCardTimeout
	}
	policy, err := newEndpointOriginPolicy(cardURL, opts.AllowedRPCOrigins)
	if err != nil {
		return nil, nil, err
	}
	baseClient := opts.httpClient()
	cardClient := restrictedHTTPClient(baseClient, policy.cardOrigins)

	cardTimeout := opts.CardTimeout
	if cardTimeout == 0 {
		cardTimeout = defaultCardTimeout
	}
	resolveCtx, cancelResolve := context.WithTimeout(ctx, cardTimeout)
	defer cancelResolve()
	resolver := agentcard.NewResolver(cardClient)
	card, err := resolver.Resolve(resolveCtx, cardURL)
	if err != nil {
		return nil, nil, fmt.Errorf("a2a.dial: resolve agent card at %q: %w", cardURL, err)
	}
	if card == nil {
		return nil, nil, ErrNilCard
	}
	if err := validateCardRPCOrigins(card, policy.rpcOrigins); err != nil {
		return nil, nil, err
	}

	rpcClient := restrictedHTTPClient(baseClient, policy.rpcOrigins)
	client, err := a2aclient.NewFromCard(ctx, card,
		a2aclient.WithJSONRPCTransport(rpcClient),
		a2aclient.WithRESTTransport(rpcClient),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("a2a.dial: open client for %q: %w", card.Name, err)
	}
	return client, card, nil
}

func validateCardRPCOrigins(card *sdka2a.AgentCard, allowed originSet) error {
	for i, iface := range card.SupportedInterfaces {
		if iface == nil {
			return fmt.Errorf("%w: supported interface %d is nil", ErrInvalidCard, i)
		}
		switch iface.ProtocolBinding {
		case sdka2a.TransportProtocolJSONRPC, sdka2a.TransportProtocolHTTPJSON:
		default:
			continue
		}
		origin, err := originFromURLString(iface.URL)
		if err != nil {
			return fmt.Errorf("%w: supported interface %d URL %q: %v", ErrInvalidCard, i, iface.URL, err)
		}
		if !allowed.contains(origin) {
			return fmt.Errorf("%w: supported interface %d uses %s", ErrOriginNotAllowed, i, origin)
		}
	}
	return nil
}
