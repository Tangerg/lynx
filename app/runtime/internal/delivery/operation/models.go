package operation

import (
	"context"

	"github.com/Tangerg/scope/app/runtime/protocol"
)

const (
	ModelsList             Name = "models.list"
	ModelsGetUtilityRole   Name = "models.getUtilityRole"
	ModelsSetUtilityRole   Name = "models.setUtilityRole"
	ModelsGetEmbeddingRole Name = "models.getEmbeddingRole"
	ModelsSetEmbeddingRole Name = "models.setEmbeddingRole"
)

func registerModels(registry *Registry) {
	registry.Query(MethodMeta{Name: ModelsList},
		func(service interface {
			ListModels(context.Context, protocol.ListModelsRequest) (*protocol.Page[protocol.Model], error)
		}, ctx context.Context, request protocol.ListModelsRequest) (*protocol.Page[protocol.Model], error) {
			return service.ListModels(ctx, request)
		})

	registry.Query(MethodMeta{Name: ModelsGetUtilityRole},
		func(service interface {
			GetUtilityRole(context.Context) (*protocol.UtilityRole, error)
		}, ctx context.Context, _ struct{}) (*protocol.UtilityRole, error) {
			return service.GetUtilityRole(ctx)
		})

	registry.Command(MethodMeta{Name: ModelsSetUtilityRole},
		func(service interface {
			SetUtilityRole(context.Context, protocol.UtilityRole) (*protocol.UtilityRole, error)
		}, ctx context.Context, request protocol.UtilityRole) (*protocol.UtilityRole, error) {
			return service.SetUtilityRole(ctx, request)
		})

	registry.Query(MethodMeta{Name: ModelsGetEmbeddingRole},
		func(service interface {
			GetEmbeddingRole(context.Context) (*protocol.EmbeddingRole, error)
		}, ctx context.Context, _ struct{}) (*protocol.EmbeddingRole, error) {
			return service.GetEmbeddingRole(ctx)
		})

	registry.Command(MethodMeta{Name: ModelsSetEmbeddingRole},
		func(service interface {
			SetEmbeddingRole(context.Context, protocol.EmbeddingRole) (*protocol.EmbeddingRole, error)
		}, ctx context.Context, request protocol.EmbeddingRole) (*protocol.EmbeddingRole, error) {
			return service.SetEmbeddingRole(ctx, request)
		})
}
