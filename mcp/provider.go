package mcp

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Tangerg/lynx/core/model/chat"
)

// NamingFunc maps a remote MCP tool to the public name reported into the
// tool registry. It must be deterministic; the same input pair must
// always yield the same output, otherwise cache invalidation may produce
// mismatched names.
type NamingFunc func(sourceName string, tool *sdkmcp.Tool) string

// DefaultNaming returns "<sourceName>_<toolName>", or the bare tool name
// when sourceName is empty. Used when ProviderConfig.Naming is left nil.
func DefaultNaming(sourceName string, tool *sdkmcp.Tool) string {
	if sourceName == "" {
		return tool.Name
	}
	return sourceName + "_" + tool.Name
}

// Source binds an initialized MCP client session to a logical name used
// by [NamingFunc] to deconflict tools across multiple servers.
type Source struct {
	// Name identifies the upstream server in tool prefixes and error
	// messages. Empty is allowed but discouraged when more than one
	// source is in play.
	Name string

	// Session is a live, initialized client session. The Provider does
	// not own the session; callers are responsible for closing it.
	Session *sdkmcp.ClientSession
}

// ProviderConfig configures a [Provider]. Sources may be empty (Tools
// then returns []); everything else has a zero-value default.
type ProviderConfig struct {
	// Sources is the list of MCP sources to discover tools from. No
	// entry may carry a nil session.
	Sources []Source

	// Naming maps each remote tool descriptor to its public name. Nil
	// means [DefaultNaming].
	Naming NamingFunc

	// MetaFunc is applied to every Tool the Provider produces. Nil
	// forwards no metadata on tool calls.
	MetaFunc MetaFunc
}

// ApplyDefaults fills the naming function when nil.
func (c *ProviderConfig) ApplyDefaults() {
	if c.Naming == nil {
		c.Naming = DefaultNaming
	}
}

// Validate checks required fields. Pure check — pair with
// [ProviderConfig.ApplyDefaults].
func (c *ProviderConfig) Validate() error {
	for i, src := range c.Sources {
		if src.Session == nil {
			return fmt.Errorf("mcp.ProviderConfig: source[%d] %q: %w", i, src.Name, ErrNilSession)
		}
	}
	return nil
}

// Provider discovers tools across one or more MCP servers and exposes
// them as []chat.Tool. The list is cached; the cache is invalidated on
// demand ([Provider.Invalidate]) or via the SDK's tools/list_changed
// notification when the caller wires [Provider.OnToolListChanged] into
// ClientOptions.
type Provider struct {
	cfg ProviderConfig

	// cache holds the latest published tool list, or nil when the cache
	// is empty/stale.
	cache atomic.Pointer[[]chat.Tool]

	// refreshMu serializes refresh so concurrent callers see at most
	// one in-flight RPC fan-out.
	refreshMu sync.Mutex
}

// NewProvider creates a Provider from cfg. cfg.Sources may be empty;
// the returned Provider then exposes an empty tool list.
func NewProvider(cfg ProviderConfig) (*Provider, error) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	// Defensive copy so post-construction mutation by the caller cannot
	// affect the Provider.
	return &Provider{cfg: ProviderConfig{
		Sources:  slices.Clone(cfg.Sources),
		Naming:   cfg.Naming,
		MetaFunc: cfg.MetaFunc,
	}}, nil
}

// Tools returns the cached tool list, fetching it on first use.
// Concurrent callers see at most one in-flight refresh; the returned
// slice is owned by the Provider and must not be mutated.
func (p *Provider) Tools(ctx context.Context) ([]chat.Tool, error) {
	if cached := p.cache.Load(); cached != nil {
		return *cached, nil
	}
	return p.refresh(ctx)
}

// Invalidate marks the cache stale; the next [Provider.Tools] call
// refetches. Safe to call from notification handlers — the actual fetch
// is deferred to the next consumer so the SDK's notification dispatcher
// is never blocked.
//
// Known (accepted) race: an Invalidate that lands while a refresh is
// mid-fetch is overwritten when that refresh stores its — by then
// stale — result. The list stays stale until the next Invalidate or
// restart. Tool lists change rarely and every change re-notifies, so
// the window self-heals; a generation counter could close it but
// isn't worth the machinery in a thin wrapper.
func (p *Provider) Invalidate() { p.cache.Store(nil) }

// OnToolListChanged is the dispatcher target for
// [sdkmcp.ClientOptions.ToolListChangedHandler]. Use it as a method
// value:
//
//	sdkmcp.NewClient(impl, &sdkmcp.ClientOptions{
//	    ToolListChangedHandler: provider.OnToolListChanged,
//	})
//
// The handler runs on the SDK dispatch goroutine and must not block, so
// it only flips the cache flag; the next Tools call performs the
// refetch.
func (p *Provider) OnToolListChanged(context.Context, *sdkmcp.ToolListChangedRequest) {
	p.Invalidate()
}

func (p *Provider) refresh(ctx context.Context) ([]chat.Tool, error) {
	p.refreshMu.Lock()
	defer p.refreshMu.Unlock()

	// Double-checked: another goroutine may have populated the cache
	// while waiting on the lock.
	if cached := p.cache.Load(); cached != nil {
		return *cached, nil
	}

	var all []chat.Tool
	seen := make(map[string]struct{})
	for _, src := range p.cfg.Sources {
		for descriptor, err := range src.Session.Tools(ctx, nil) {
			if err != nil {
				return nil, fmt.Errorf("mcp.Provider.refresh: list tools from source %q: %w", src.Name, err)
			}

			tool, err := NewTool(ToolConfig{
				Session:      src.Session,
				Descriptor:   descriptor,
				PrefixedName: p.cfg.Naming(src.Name, descriptor),
				MetaFunc:     p.cfg.MetaFunc,
			})
			if err != nil {
				return nil, fmt.Errorf("mcp.Provider.refresh: wrap tool %q from source %q: %w", descriptor.Name, src.Name, err)
			}

			name := tool.Definition().Name
			if _, dup := seen[name]; dup {
				return nil, fmt.Errorf("mcp.Provider.refresh: duplicate tool name after prefixing: %q", name)
			}
			seen[name] = struct{}{}
			all = append(all, tool)
		}
	}

	all = slices.Clip(all) // so callers can't append into the shared backing array
	p.cache.Store(&all)
	return all, nil
}
