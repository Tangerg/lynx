package operation

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func registerModels(registry *Registry) {
	Query(registry, MethodMeta{Name: "models.list"},
		func(service interface {
			ListModels(context.Context, protocol.ListModelsRequest) (*protocol.Page[protocol.Model], error)
		}, ctx context.Context, request protocol.ListModelsRequest) (*protocol.Page[protocol.Model], error) {
			return service.ListModels(ctx, request)
		})

	Query(registry, MethodMeta{Name: "models.getUtilityRole"},
		func(service interface {
			GetUtilityRole(context.Context) (*protocol.UtilityRole, error)
		}, ctx context.Context, _ struct{}) (*protocol.UtilityRole, error) {
			return service.GetUtilityRole(ctx)
		})

	Command(registry, MethodMeta{Name: "models.setUtilityRole"},
		func(service interface {
			SetUtilityRole(context.Context, protocol.UtilityRole) (*protocol.UtilityRole, error)
		}, ctx context.Context, request protocol.UtilityRole) (*protocol.UtilityRole, error) {
			return service.SetUtilityRole(ctx, request)
		})

	Query(registry, MethodMeta{Name: "models.getEmbeddingRole"},
		func(service interface {
			GetEmbeddingRole(context.Context) (*protocol.EmbeddingRole, error)
		}, ctx context.Context, _ struct{}) (*protocol.EmbeddingRole, error) {
			return service.GetEmbeddingRole(ctx)
		})

	Command(registry, MethodMeta{Name: "models.setEmbeddingRole"},
		func(service interface {
			SetEmbeddingRole(context.Context, protocol.EmbeddingRole) (*protocol.EmbeddingRole, error)
		}, ctx context.Context, request protocol.EmbeddingRole) (*protocol.EmbeddingRole, error) {
			return service.SetEmbeddingRole(ctx, request)
		})
}
