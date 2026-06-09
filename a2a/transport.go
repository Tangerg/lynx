package a2a

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	sdka2a "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/a2aproject/a2a-go/v2/a2aclient/agentcard"

	"github.com/Tangerg/lynx/core/model/chat"
)

// ClientConfig declares how to reach one remote A2A agent. It mirrors the
// declarative mcp.ServerConfig: a logical name plus a card URL the resolver
// reads .well-known/agent-card.json from.
type ClientConfig struct {
	// Name is the logical handle for the agent — used as the lynx tool name
	// when this config is turned into a [chat.Tool]. Empty defaults to a
	// sanitized form of the resolved AgentCard's Name.
	Name string

	// CardURL is the base URL the AgentCard is resolved from (the resolver
	// appends the well-known path). Required.
	CardURL string

	// HTTPClient is the client used for both card resolution and RPC calls.
	// nil uses a default client.
	HTTPClient *http.Client
}

// Validate reports whether the config can be dialed.
func (c *ClientConfig) Validate() error {
	if c.CardURL == "" {
		return ErrEmptyCardURL
	}
	return nil
}

// httpClient returns the HTTP client to use, defaulting to the shared
// http.DefaultClient when none was supplied.
func (c *ClientConfig) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

// Dial resolves the remote AgentCard at cfg.CardURL and opens a client
// against it. The client negotiates a transport from the card's advertised
// interfaces; JSON-RPC and REST (both HTTP) are registered, matching the
// rest of the stack — a card that only advertises gRPC will fail to dial.
//
// The returned card is the resolved descriptor (used to build the tool's
// definition). Callers own the client and must Destroy it when done.
func Dial(ctx context.Context, cfg ClientConfig) (*a2aclient.Client, *sdka2a.AgentCard, error) {
	if err := cfg.Validate(); err != nil {
		return nil, nil, err
	}
	httpClient := cfg.httpClient()

	resolver := agentcard.NewResolver(httpClient)
	card, err := resolver.Resolve(ctx, cfg.CardURL)
	if err != nil {
		return nil, nil, fmt.Errorf("a2a.Dial: resolve agent card at %q: %w", cfg.CardURL, err)
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

// DialAll resolves and connects every config and wraps each remote agent as a
// [chat.Tool] — the one-call client setup mirroring lyra's MCP wiring. Unlike
// MCP there is no per-agent tool list to refresh (each remote agent is a single
// static capability), so the tools are built once here rather than behind a
// stateful provider.
//
// It returns the opened clients alongside the tools so the caller can Destroy
// them on shutdown via [CloseClients]. Two configs resolving to the same tool
// name is an error — set [ClientConfig.Name] to disambiguate. On any failure it
// closes the clients dialed so far and returns the error (no leak).
func DialAll(ctx context.Context, configs ...ClientConfig) ([]chat.Tool, []*a2aclient.Client, error) {
	clients := make([]*a2aclient.Client, 0, len(configs))
	tools := make([]chat.Tool, 0, len(configs))
	seen := make(map[string]struct{}, len(configs))
	for _, cfg := range configs {
		client, card, err := Dial(ctx, cfg)
		if err != nil {
			_ = CloseClients(clients) // best-effort cleanup; the dial error is what matters
			return nil, nil, err
		}
		clients = append(clients, client)

		tool, err := NewAgentTool(AgentToolConfig{Client: client, Card: card, Name: cfg.Name})
		if err != nil {
			_ = CloseClients(clients)
			return nil, nil, err
		}
		name := tool.Definition().Name
		if _, dup := seen[name]; dup {
			_ = CloseClients(clients)
			return nil, nil, fmt.Errorf("a2a.DialAll: duplicate tool name %q (set ClientConfig.Name to disambiguate)", name)
		}
		seen[name] = struct{}{}
		tools = append(tools, tool)
	}
	return tools, clients, nil
}

// CloseClients destroys a set of clients, joining any errors — the shutdown
// counterpart to [DialAll]'s returned client slice.
func CloseClients(clients []*a2aclient.Client) error {
	var errs []error
	for _, client := range clients {
		if err := client.Destroy(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
