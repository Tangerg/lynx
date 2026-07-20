package runtime

import (
	"context"
	"fmt"
	"slices"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/internal/panicerr"
	"github.com/Tangerg/lynx/tools"
)

// engineExtensions exposes the engine-scoped extension list.
func (p *Process) engineExtensions() []core.Extension {
	if p.engine == nil {
		return nil
	}
	return p.engine.extensions.list
}

// processExtensions exposes the per-process extension list (from
// [core.ProcessOptions.Extensions]).
func (p *Process) processExtensions() []core.Extension {
	if p.options == nil {
		return nil
	}
	return p.options.extensions
}

// childExtensions propagates a parent's process-scope [EventListener]
// extensions onto a child's option set so the whole delegation subtree feeds
// the listener the parent registered. Other capabilities stay scoped to the
// process that declared them. A child-declared listener with the same Name
// wins, and duplicates are skipped so process extension validation remains
// deterministic.
func (p *Process) childExtensions(childExtensions []core.Extension) []core.Extension {
	childExtensions = slices.Clone(childExtensions)
	if p == nil || p.options == nil || len(p.options.extensions) == 0 {
		return childExtensions
	}
	seen := make(map[string]struct{}, len(childExtensions))
	for _, extension := range childExtensions {
		if !valueIsNil(extension) {
			seen[extension.Name()] = struct{}{}
		}
	}
	for _, extension := range p.options.extensions {
		if valueIsNil(extension) {
			continue
		}
		if _, ok := extension.(EventListener); !ok {
			continue
		}
		if _, duplicate := seen[extension.Name()]; duplicate {
			continue
		}
		childExtensions = append(childExtensions, extension)
		seen[extension.Name()] = struct{}{}
	}
	return childExtensions
}

// combinedExtensions returns engine extensions followed by process
// extensions — the natural ordering for onion / wrap chains where
// engine sits outermost (registered earliest) and process sits
// innermost (registered last). Goal-approver dispatch reads this list.
func (p *Process) combinedExtensions() []core.Extension {
	return mergeExtensions(p.engineExtensions(), p.processExtensions())
}

// combinedExtensionsResolverFirst returns process extensions BEFORE
// engine extensions — the order used for first-hit resolvers so a
// process-scope override is consulted first.
func (p *Process) combinedExtensionsResolverFirst() []core.Extension {
	return mergeExtensions(p.processExtensions(), p.engineExtensions())
}

// mergeExtensions concatenates first then second, returning the input
// directly (no allocation) when either side is empty.
func mergeExtensions(first, second []core.Extension) []core.Extension {
	if len(second) == 0 {
		return first
	}
	if len(first) == 0 {
		return second
	}
	merged := make([]core.Extension, 0, len(first)+len(second))
	merged = append(merged, first...)
	merged = append(merged, second...)
	return merged
}

// toolResolverFor builds the action-scoped resolver exposed by ProcessContext.
// Process extensions resolve first, while middleware wraps engine-first so the
// process-scoped decorator is outermost.
func (p *Process) toolResolverFor(action core.Action) func(context.Context, []core.ToolGroupRequirement) ([]tools.Tool, error) {
	resolvers := collectExtensions[core.ToolGroupResolver](p.combinedExtensionsResolverFirst())
	middleware := collectExtensions[core.ToolMiddleware](p.combinedExtensions())
	if len(resolvers) == 0 {
		return nil
	}
	return func(ctx context.Context, requirements []core.ToolGroupRequirement) ([]tools.Tool, error) {
		var resolved []tools.Tool

		for _, requirement := range requirements {
			group, found, err := runToolGroupResolvers(ctx, resolvers, requirement)
			if err != nil {
				return nil, fmt.Errorf("resolve tools for role %q: %w", requirement.Role, err)
			}
			if !found {
				continue
			}

			groupTools, err := loadToolGroup(ctx, group, requirement.Role)
			if err != nil {
				return nil, fmt.Errorf("load tools for role %q: %w", requirement.Role, err)
			}
			for _, tool := range groupTools {
				wrapped, err := p.wrapTool(middleware, action, tool)
				if err != nil {
					return nil, fmt.Errorf("wrap tool for role %q: %w", requirement.Role, err)
				}
				resolved = append(resolved, wrapped)
			}
		}
		return resolved, nil
	}
}

func loadToolGroup(ctx context.Context, group core.ToolGroup, role string) (groupTools []tools.Tool, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = panicerr.New(fmt.Sprintf("tool group %q Tools panicked", role), recovered)
		}
	}()
	return group.Tools(ctx)
}
