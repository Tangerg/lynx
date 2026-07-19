package toolloop

import (
	"context"
	"sync"

	"github.com/Tangerg/lynx/core/chat"
)

// toolPromotions collects the tool definitions a running tool asked the loop to
// advertise for the remainder of the interaction. The runner drains it into the
// request's advertised toolset after each concurrency segment, so a tool the
// model discovers mid-loop — e.g. a search_tools meta-tool over a large MCP
// catalog the initial manifest deliberately withheld — becomes directly callable
// on the next model round without ever re-listing the whole catalog up front.
//
// A segment runs its calls under the runner's bounded errgroup, so several tools
// may promote in parallel; add is therefore mutex-guarded.
type toolPromotions struct {
	mu      sync.Mutex
	pending []chat.ToolDefinition
}

func (p *toolPromotions) add(defs []chat.ToolDefinition) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pending = append(p.pending, defs...)
}

// drain returns the promotions collected since the last drain and clears them.
func (p *toolPromotions) drain() []chat.ToolDefinition {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.pending) == 0 {
		return nil
	}
	out := p.pending
	p.pending = nil
	return out
}

type promotionsContextKey struct{}

func withPromotions(ctx context.Context, sink *toolPromotions) context.Context {
	return context.WithValue(ctx, promotionsContextKey{}, sink)
}

// PromoteTools asks the running tool loop to advertise defs to the model for the
// remainder of the interaction. A tool calls this when it resolves a capability
// the initial request deliberately kept out of the manifest: the definitions
// join the advertised toolset for every subsequent model round, and survive a
// pause/resume because the runner folds them into the request before it snapshots
// the checkpoint.
//
// Each definition must name a tool the interaction's [ToolResolver] can resolve
// to a matching definition (advertised ⊆ resolvable, unadvertised until
// promoted). The runner only advertises; it never registers executables, so a
// deferred-then-promoted tool must already live in the resolver. Definitions that
// are invalid, already advertised, or not resolvable to a matching tool are
// dropped when the runner merges — promotion cannot smuggle an unexecutable name
// into the manifest.
//
// Calling this outside a running Runner (no sink bound — e.g. a unit test that
// invokes the tool directly) is a no-op: the tool's own result is unaffected,
// only the ambient advertise-more capability is absent.
func PromoteTools(ctx context.Context, defs ...chat.ToolDefinition) {
	if ctx == nil || len(defs) == 0 {
		return
	}
	sink, ok := ctx.Value(promotionsContextKey{}).(*toolPromotions)
	if !ok || sink == nil {
		return
	}
	sink.add(defs)
}
