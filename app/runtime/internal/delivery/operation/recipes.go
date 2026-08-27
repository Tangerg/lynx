package operation

import (
	"context"

	"github.com/Tangerg/scope/app/runtime/protocol"
)

const RecipesList Name = "recipes.list"

func registerRecipes(registry *Registry) {
	registry.Query(MethodMeta{
		Name:   RecipesList,
		Errors: []string{protocol.ErrWorkspaceUnavailable.Error()},
	}, func(service interface {
		ListRecipes(context.Context, protocol.WorkspaceQuery) (*protocol.Page[protocol.Recipe], error)
	}, ctx context.Context, request protocol.WorkspaceQuery) (*protocol.Page[protocol.Recipe], error) {
		return service.ListRecipes(ctx, request)
	})
}
