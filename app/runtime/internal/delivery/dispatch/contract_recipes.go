package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func registerRecipes(r *Registry) {
	Query(r, MethodMeta{
		Name:      "recipes.list",
		Errors:    []string{protocol.ErrWorkspaceUnavailable.Error()},
		Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.WorkspaceQuery) (*protocol.Page[protocol.Recipe], error) {
		return d.api.ListRecipes(ctx, in)
	})
}
