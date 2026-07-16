package runtime

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/tools"
)

// toolResolverFor returns the ResolveTools closure used by ProcessContext.
// nil action is allowed (the resolver still works; ToolMiddlewares receive
// nil action and should treat it as "outside an action body").
//
// Resolvers are walked process-first so a process-scope override beats the
// engine default; tool middleware wraps engine-first then process-last so a
// process-scope decorator is the outermost wrap and runs after engine
// middleware.
func (p *Process) toolResolverFor(action core.Action) func(context.Context, []core.ToolGroupRequirement) ([]tools.Tool, error) {
	resolvers := collectExtensions[core.ToolGroupResolver](p.combinedExtensionsResolverFirst())
	middleware := collectExtensions[core.ToolMiddleware](p.combinedExtensions())
	if len(resolvers) == 0 {
		return nil
	}
	return func(ctx context.Context, requirements []core.ToolGroupRequirement) ([]tools.Tool, error) {
		var resolved []tools.Tool

		for _, requirement := range requirements {
			group, found, err := runToolGroupResolvers(resolvers, ctx, requirement)
			if err != nil {
				return nil, fmt.Errorf("resolve tools for role %q: %w", requirement.Role, err)
			}
			if !found {
				continue
			}

			groupTools, err := group.Tools(ctx)
			if err != nil {
				return nil, fmt.Errorf("load tools for role %q: %w", requirement.Role, err)
			}
			for _, tool := range groupTools {
				resolved = append(resolved, wrapTool(middleware, p, action, tool))
			}
		}
		return resolved, nil
	}
}
