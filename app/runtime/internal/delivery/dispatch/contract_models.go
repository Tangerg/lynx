package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func registerModels(r *Registry) {
	Query(r, MethodMeta{Name: "models.list", Stability: stable},
		func(d *Router, ctx context.Context, in protocol.ListModelsRequest) (*protocol.Page[protocol.Model], error) {
			return d.api.ListModels(ctx, in)
		})

	Query(r, MethodMeta{Name: "models.getUtilityRole", Stability: stable},
		func(d *Router, ctx context.Context, _ struct{}) (*protocol.UtilityRole, error) {
			return d.api.GetUtilityRole(ctx)
		})

	Command(r, MethodMeta{Name: "models.setUtilityRole", Stability: stable},
		func(d *Router, ctx context.Context, in protocol.UtilityRole) (*protocol.UtilityRole, error) {
			return d.api.SetUtilityRole(ctx, in)
		})

	Query(r, MethodMeta{Name: "models.getEmbeddingRole", Stability: stable},
		func(d *Router, ctx context.Context, _ struct{}) (*protocol.EmbeddingRole, error) {
			return d.api.GetEmbeddingRole(ctx)
		})

	Command(r, MethodMeta{Name: "models.setEmbeddingRole", Stability: stable},
		func(d *Router, ctx context.Context, in protocol.EmbeddingRole) (*protocol.EmbeddingRole, error) {
			return d.api.SetEmbeddingRole(ctx, in)
		})
}
