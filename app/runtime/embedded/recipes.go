package embedded

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// ListRecipes returns recipes applicable to a workspace.
func (r *Runtime) ListRecipes(ctx context.Context, request protocol.WorkspaceQuery, options CallOptions) (*protocol.Page[protocol.Recipe], error) {
	return invoke[protocol.WorkspaceQuery, *protocol.Page[protocol.Recipe]](ctx, r, "recipes.list", request, callOptions(options))
}
