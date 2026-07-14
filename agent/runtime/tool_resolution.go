package runtime

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/tools"
)

// toolResolverFor returns the ResolveTools closure used by ProcessContext.
// nil action is allowed (the resolver still works; ToolDecorators receive
// nil action and should treat it as "outside an action body").
//
// Resolvers are walked process-first so a process-scope override beats the
// platform default; decorators wrap platform-first then process-last so a
// process-scope decorator is the outermost wrap and runs after platform
// decorators.
func (p *AgentProcess) toolResolverFor(action core.Action) core.ToolResolver {
	resolvers := collectExtensions[core.ToolGroupResolver](p.combinedExtensionsResolverFirst())
	decorators := collectExtensions[core.ToolDecorator](p.combinedExtensions())
	if len(resolvers) == 0 {
		return nil
	}
	return func(ctx context.Context, requirements []core.ToolGroupRequirement) ([]tools.Tool, error) {
		var collected []tools.Tool

		for _, req := range requirements {
			group, found, err := runToolGroupResolvers(resolvers, ctx, req)
			if err != nil {
				return nil, fmt.Errorf("resolve tools for role %q: %w", req.Role, err)
			}
			if !found {
				continue
			}

			tools, err := group.Tools(ctx)
			if err != nil {
				return nil, fmt.Errorf("load tools for role %q: %w", req.Role, err)
			}
			for _, tool := range tools {
				collected = append(collected, runToolDecorators(decorators, p, action, tool))
			}
		}
		return collected, nil
	}
}
