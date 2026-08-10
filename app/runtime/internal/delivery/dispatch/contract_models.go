package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func registerModels(registry *Registry) {
	Query(registry, MethodMeta{Name: "models.list", Stability: stable},
		func(router *Router, ctx context.Context, request protocol.ListModelsRequest) (*protocol.Page[protocol.Model], error) {
			return router.api.ListModels(ctx, request)
		})

	Query(registry, MethodMeta{Name: "models.getUtilityRole", Stability: stable},
		func(router *Router, ctx context.Context, _ struct{}) (*protocol.UtilityRole, error) {
			return router.api.GetUtilityRole(ctx)
		})

	Command(registry, MethodMeta{Name: "models.setUtilityRole", Stability: stable},
		func(router *Router, ctx context.Context, request protocol.UtilityRole) (*protocol.UtilityRole, error) {
			return router.api.SetUtilityRole(ctx, request)
		})

	Query(registry, MethodMeta{Name: "models.getEmbeddingRole", Stability: stable},
		func(router *Router, ctx context.Context, _ struct{}) (*protocol.EmbeddingRole, error) {
			return router.api.GetEmbeddingRole(ctx)
		})

	Command(registry, MethodMeta{Name: "models.setEmbeddingRole", Stability: stable},
		func(router *Router, ctx context.Context, request protocol.EmbeddingRole) (*protocol.EmbeddingRole, error) {
			return router.api.SetEmbeddingRole(ctx, request)
		})
}
