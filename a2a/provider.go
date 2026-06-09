package a2a

import (
	"context"
	"errors"
	"fmt"

	sdka2a "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"

	"github.com/Tangerg/lynx/core/model/chat"
)

// Source is one remote A2A agent the [Provider] exposes as a tool. Client
// and Card come from [Dial]; Name overrides the tool name (empty derives it
// from the card).
type Source struct {
	Client *a2aclient.Client
	Card   *sdka2a.AgentCard
	Name   string
}

// Tool wraps this source's remote agent as a [chat.Tool].
func (s Source) Tool() (*AgentTool, error) {
	return NewAgentTool(AgentToolConfig{Client: s.Client, Card: s.Card, Name: s.Name})
}

// Provider aggregates several remote A2A agents into one []chat.Tool — the
// client-side analogue of mcp.Provider. Unlike MCP there is no per-server
// tool list to refresh: each remote agent is a single capability, so the
// tools are built once at construction.
type Provider struct {
	tools []chat.Tool
}

// NewProvider wraps each source as an [AgentTool]. It errors if a source is
// missing its client or card, or if two sources resolve to the same tool
// name (an ambiguous registry the caller must disambiguate via Source.Name).
func NewProvider(sources ...Source) (*Provider, error) {
	tools := make([]chat.Tool, 0, len(sources))
	seen := make(map[string]struct{}, len(sources))
	for _, src := range sources {
		tool, err := src.Tool()
		if err != nil {
			return nil, err
		}
		name := tool.Definition().Name
		if _, dup := seen[name]; dup {
			return nil, fmt.Errorf("a2a.NewProvider: duplicate tool name %q (set Source.Name to disambiguate)", name)
		}
		seen[name] = struct{}{}
		tools = append(tools, tool)
	}
	return &Provider{tools: tools}, nil
}

// Tools returns the wrapped remote agents as lynx tools.
func (p *Provider) Tools() []chat.Tool { return p.tools }

// DialAll resolves and connects every config, then aggregates the results
// into a Provider — the one-call client setup mirroring lyra's MCP wiring.
// It returns the opened clients alongside the provider so the caller can
// Destroy them on shutdown. On any failure it closes the clients dialed so
// far and returns the error (no leak).
func DialAll(ctx context.Context, configs ...ClientConfig) (*Provider, []*a2aclient.Client, error) {
	clients := make([]*a2aclient.Client, 0, len(configs))
	sources := make([]Source, 0, len(configs))
	for _, cfg := range configs {
		client, card, err := Dial(ctx, cfg)
		if err != nil {
			_ = CloseClients(clients) // best-effort cleanup; the dial error is what matters
			return nil, nil, err
		}
		clients = append(clients, client)
		sources = append(sources, Source{Client: client, Card: card, Name: cfg.Name})
	}

	provider, err := NewProvider(sources...)
	if err != nil {
		_ = CloseClients(clients)
		return nil, nil, err
	}
	return provider, clients, nil
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
