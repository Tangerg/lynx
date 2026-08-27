package embedded

import (
	"context"

	"github.com/Tangerg/scope/app/runtime/internal/delivery/operation"
	"github.com/Tangerg/scope/app/runtime/protocol"
)

// ListRecipes returns recipes applicable to a workspace.
func (r *Runtime) ListRecipes(ctx context.Context, request protocol.WorkspaceQuery, options CallOptions) (*protocol.Page[protocol.Recipe], error) {
	return r.invoke[protocol.WorkspaceQuery, *protocol.Page[protocol.Recipe]](ctx, operation.RecipesList, request, callOptions(options))
}
