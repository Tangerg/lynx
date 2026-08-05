package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func registerProviders(r *Registry) {
	Query(r, MethodMeta{Name: "providers.list", Stability: stable},
		func(d *Router, ctx context.Context, _ struct{}) (*protocol.Page[protocol.Provider], error) {
			return d.api.ListProviders(ctx)
		})

	Command(r, MethodMeta{Name: "providers.update", Stability: stable},
		func(d *Router, ctx context.Context, in protocol.UpdateProviderRequest) (*protocol.Provider, error) {
			return d.api.UpdateProvider(ctx, in)
		})

	// The probe's verdict rides its own result, so the call succeeds even when
	// the provider does not; the read persists nothing and needs no replay guard.
	Query(r, MethodMeta{Name: "providers.test", Stability: stable},
		func(d *Router, ctx context.Context, in protocol.TestProviderRequest) (*protocol.ProviderTestResult, error) {
			return d.api.TestProvider(ctx, in.Provider)
		})
}
