package toolloop

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/Tangerg/lynx/agent/internal/nilvalue"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tool"
)

// toolPromotions collects the tool definitions a running tool asked the loop to
// advertise for the remainder of the interaction. The runner drains it into the
// request's advertised toolset after each concurrency batch, so a tool the
// model discovers mid-loop — e.g. a search_tools meta-tool over a large MCP
// catalog the initial manifest deliberately withheld — becomes directly callable
// on the next model round without ever re-listing the whole catalog up front.
//
// A batch runs its calls under the runner's bounded errgroup, so several tools
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
// [InitialManifest] is the matching half: it builds the initial manifest that leaves
// deferred tools out.
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

// ToolDeferrer is the optional capability a tool implements to keep other
// resolvable tools out of the model's initial manifest. It is the withholding
// half of [PromoteTools]: the named tools stay executable through the
// interaction's [ToolResolver], but the model does not see them until this tool
// promotes the ones it picked.
//
// Like [ConcurrentTool] it lives here rather than in tools, so a tool can state
// the intent without depending on a particular loop driver, and a driver that
// ignores the advice stays correct — it simply advertises everything up front.
type ToolDeferrer interface {
	// DeferredToolNames reports the tool names this tool withholds. Names that
	// do not appear among the candidates are ignored.
	DeferredToolNames() []string
}

// InitialManifest returns the initial manifest for candidates: one definition per
// tool, minus every name a [ToolDeferrer] among them withholds. Withheld tools
// are absent from the manifest but still resolvable, which is what makes a
// mid-loop [PromoteTools] meaningful — advertising is the only thing promotion
// changes.
//
// Definitions are cloned and ordered by name, so one candidate set yields one
// manifest regardless of iteration order. Callers that want to advertise a
// different subset can build the manifest themselves; this is the projection
// that matches promotion. A malformed wrapping chain is returned as
// [tool.ErrInvalidWrappingChain].
func InitialManifest(candidates []tool.Tool) ([]chat.ToolDefinition, error) {
	var withheld map[string]struct{}
	for _, candidate := range candidates {
		deferring, ok, err := tool.Capability[ToolDeferrer](candidate)
		if err != nil {
			return nil, fmt.Errorf("toolloop.InitialManifest: %w", err)
		}
		if !ok {
			continue
		}
		names, err := hostedTool{tool: candidate}.deferredNames(deferring)
		if err != nil {
			return nil, err
		}
		for _, name := range names {
			if withheld == nil {
				withheld = make(map[string]struct{})
			}
			withheld[name] = struct{}{}
		}
	}

	manifest := make([]chat.ToolDefinition, 0, len(candidates))
	for _, candidate := range candidates {
		if nilvalue.Is(candidate) {
			continue
		}
		definition, err := hostedTool{tool: candidate}.definition()
		if err != nil {
			return nil, err
		}
		if _, hidden := withheld[definition.Name]; hidden {
			continue
		}
		manifest = append(manifest, definition.Clone())
	}
	slices.SortFunc(manifest, func(left, right chat.ToolDefinition) int {
		return strings.Compare(left.Name, right.Name)
	})
	return manifest, nil
}
