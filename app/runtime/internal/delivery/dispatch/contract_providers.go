package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func registerProviders(registry *Registry) {
	Query(registry, MethodMeta{Name: "providers.list", Stability: stable},
		func(router *Router, ctx context.Context, _ struct{}) (*protocol.Page[protocol.Provider], error) {
			return router.api.ListProviders(ctx)
		})

	Command(registry, MethodMeta{Name: "providers.update", Stability: stable},
		func(router *Router, ctx context.Context, request protocol.UpdateProviderRequest) (*protocol.Provider, error) {
			return router.api.UpdateProvider(ctx, request)
		})

	// The probe's verdict rides its own result, so the call succeeds even when
	// the provider does not; the read persists nothing and needs no replay guard.
	Query(registry, MethodMeta{Name: "providers.test", Stability: stable},
		func(router *Router, ctx context.Context, request protocol.TestProviderRequest) (*protocol.ProviderTestResult, error) {
			return router.api.TestProvider(ctx, request.Provider)
		})
}
