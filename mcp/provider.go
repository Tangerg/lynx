package mcp

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Tangerg/lynx/core/model/chat"
)

// Source binds an initialized MCP client session with a logical name used by
// NamingFunc to deconflict tools across multiple servers.
type Source struct {
	// Name identifies the upstream server for naming and error messages.
	// Empty is allowed but discouraged once more than one server is in play.
	Name string

	// Session is a live, initialized client session. The Provider does not
	// own the session; callers are responsible for closing it.
	Session *sdkmcp.ClientSession
}

// ProviderConfig configures a Provider. The Sources list may be empty
// (Tools then returns []); everything else has a zero-value default. Call
// Validate before use to surface missing fields and apply defaults in
// place.
type ProviderConfig struct {
	// Sources is the list of MCP sources to discover tools from. May be
	// empty, but no entry may carry a nil session.
	Sources []Source

	// Naming maps each remote tool descriptor to the public name reported
	// to lynx. Nil means DefaultNaming; Validate fills it in.
	Naming NamingFunc

	// MetaFunc is applied to every Tool the Provider produces. Nil means
	// no metadata is forwarded on tool calls.
	MetaFunc MetaFunc
}

// Validate checks required fields and applies defaults in place. It is
// safe to call multiple times.
func (c *ProviderConfig) Validate() error {
	if c == nil {
		return errors.New("provider config must not be nil")
	}
	for i, src := range c.Sources {
		if src.Session == nil {
			return fmt.Errorf("provider config: source[%d] %q: session must not be nil", i, src.Name)
		}
	}
	if c.Naming == nil {
		c.Naming = DefaultNaming
	}
	return nil
}

// Provider discovers tools across one or more MCP servers and exposes them
// as a list of chat.Tool. The list is cached; the cache is invalidated on
// demand (Invalidate) or via the SDK's tools/list_changed notification when
// the caller wires OnToolListChanged into ClientOptions.
type Provider struct {
	cfg ProviderConfig

	// cache holds the latest published tool list, or nil when the cache is
	// empty/stale.
	cache atomic.Pointer[[]chat.Tool]

	// refreshMu serializes refresh so concurrent callers see at most one
	// in-flight RPC fan-out.
	refreshMu sync.Mutex
}

// NewProvider creates a Provider from the supplied configuration. cfg.Sources
// may be empty; the returned Provider then exposes an empty tool list.
func NewProvider(cfg ProviderConfig) (*Provider, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// Defensive copy of the source slice so post-construction mutation by
	// the caller cannot affect the Provider.
	cfg.Sources = append([]Source(nil), cfg.Sources...)

	return &Provider{cfg: cfg}, nil
}

// Tools returns the cached tool list, fetching it on first use. Concurrent
// callers see at most one in-flight refresh; the returned slice is owned by
// the Provider and must not be mutated.
func (p *Provider) Tools(ctx context.Context) ([]chat.Tool, error) {
	if cached := p.cache.Load(); cached != nil {
		return *cached, nil
	}
	return p.refresh(ctx)
}

// Invalidate marks the cache stale; the next Tools call will refetch.
//
// Safe to call from notification handlers: the actual fetch is deferred to
// the next consumer so the SDK's notification dispatcher is never blocked.
func (p *Provider) Invalidate() { p.cache.Store(nil) }

// OnToolListChanged is the dispatcher target for
// sdkmcp.ClientOptions.ToolListChangedHandler. Use it as a method value:
//
//	sdkmcp.NewClient(impl, &sdkmcp.ClientOptions{
//	    ToolListChangedHandler: provider.OnToolListChanged,
//	})
//
// The handler runs on the SDK dispatch goroutine and must not block, so it
// only flips the cache flag; the next Tools call performs the refetch.
func (p *Provider) OnToolListChanged(context.Context, *sdkmcp.ToolListChangedRequest) {
	p.Invalidate()
}

func (p *Provider) refresh(ctx context.Context) ([]chat.Tool, error) {
	p.refreshMu.Lock()
	defer p.refreshMu.Unlock()

	// Double-checked: another goroutine may have populated the cache while
	// we were waiting on the lock.
	if cached := p.cache.Load(); cached != nil {
		return *cached, nil
	}

	all := make([]chat.Tool, 0)
	for _, src := range p.cfg.Sources {
		wrapped, err := p.wrapToolsFromSource(ctx, src)
		if err != nil {
			return nil, err
		}
		all = append(all, wrapped...)
	}

	if err := validateUniqueNames(all); err != nil {
		return nil, err
	}

	p.cache.Store(&all)
	return all, nil
}

// wrapToolsFromSource lists every tool exposed by src and converts each
// descriptor into a chat.Tool using the Provider's configuration.
func (p *Provider) wrapToolsFromSource(ctx context.Context, src Source) ([]chat.Tool, error) {
	wrapped := make([]chat.Tool, 0)
	for descriptor, err := range src.Session.Tools(ctx, nil) {
		if err != nil {
			return nil, fmt.Errorf("list tools from source %q: %w", src.Name, err)
		}

		tool, err := NewTool(ToolConfig{
			Session:      src.Session,
			Descriptor:   descriptor,
			PrefixedName: p.cfg.Naming(src.Name, descriptor),
			MetaFunc:     p.cfg.MetaFunc,
		})
		if err != nil {
			return nil, fmt.Errorf("wrap tool %q from source %q: %w", descriptor.Name, src.Name, err)
		}
		wrapped = append(wrapped, tool)
	}
	return wrapped, nil
}

// validateUniqueNames returns an error when two tools share the same public
// name after the NamingFunc has been applied. Failing fast is preferred to
// silently shadowing one tool with another.
func validateUniqueNames(tools []chat.Tool) error {
	seen := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		name := tool.Definition().Name
		if _, dup := seen[name]; dup {
			return fmt.Errorf("duplicate tool name after prefixing: %q", name)
		}
		seen[name] = struct{}{}
	}
	return nil
}
