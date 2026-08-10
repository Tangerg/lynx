package operation

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func registerRecipes(registry *Registry) {
	Query(registry, MethodMeta{
		Name:      "recipes.list",
		Errors:    []string{protocol.ErrWorkspaceUnavailable.Error()},
		Stability: stable,
	}, func(service Service, ctx context.Context, request protocol.WorkspaceQuery) (*protocol.Page[protocol.Recipe], error) {
		return service.ListRecipes(ctx, request)
	})
}
