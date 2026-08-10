package operation

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func registerProviders(registry *Registry) {
	Query(registry, MethodMeta{Name: "providers.list", Stability: stable},
		func(service Service, ctx context.Context, _ struct{}) (*protocol.Page[protocol.Provider], error) {
			return service.ListProviders(ctx)
		})

	Command(registry, MethodMeta{Name: "providers.update", Stability: stable},
		func(service Service, ctx context.Context, request protocol.UpdateProviderRequest) (*protocol.Provider, error) {
			return service.UpdateProvider(ctx, request)
		})

	// The probe's verdict rides its own result, so the call succeeds even when
	// the provider does not; the read persists nothing and needs no replay guard.
	Query(registry, MethodMeta{Name: "providers.test", Stability: stable},
		func(service Service, ctx context.Context, request protocol.TestProviderRequest) (*protocol.ProviderTestResult, error) {
			return service.TestProvider(ctx, request.Provider)
		})
}
