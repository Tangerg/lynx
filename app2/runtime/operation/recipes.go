package operation

import (
	"context"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

func registerRecipes(registry *Registry) {
	Query(registry, MethodMeta{
		Name:   "recipes.list",
		Errors: []string{protocol.ErrWorkspaceUnavailable.Error()},
	}, func(service interface {
		ListRecipes(context.Context, protocol.WorkspaceQuery) (*protocol.Page[protocol.Recipe], error)
	}, ctx context.Context, request protocol.WorkspaceQuery) (*protocol.Page[protocol.Recipe], error) {
		return service.ListRecipes(ctx, request)
	})
}
