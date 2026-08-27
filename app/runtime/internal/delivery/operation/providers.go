package operation

import (
	"context"

	"github.com/Tangerg/scope/app/runtime/protocol"
)

const (
	ProvidersList   Name = "providers.list"
	ProvidersUpdate Name = "providers.update"
	ProvidersTest   Name = "providers.test"
)

func registerProviders(registry *Registry) {
	registry.Query(MethodMeta{Name: ProvidersList},
		func(service interface {
			ListProviders(context.Context) (*protocol.Page[protocol.Provider], error)
		}, ctx context.Context, _ struct{}) (*protocol.Page[protocol.Provider], error) {
			return service.ListProviders(ctx)
		})

	registry.Command(MethodMeta{Name: ProvidersUpdate},
		func(service interface {
			UpdateProvider(context.Context, protocol.UpdateProviderRequest) (*protocol.Provider, error)
		}, ctx context.Context, request protocol.UpdateProviderRequest) (*protocol.Provider, error) {
			return service.UpdateProvider(ctx, request)
		})

	// The probe's verdict rides its own result, so the call succeeds even when
	// the provider does not; the read persists nothing and needs no replay guard.
	registry.Query(MethodMeta{Name: ProvidersTest},
		func(service interface {
			TestProvider(context.Context, string) (*protocol.ProviderTestResult, error)
		}, ctx context.Context, request protocol.TestProviderRequest) (*protocol.ProviderTestResult, error) {
			return service.TestProvider(ctx, request.Provider)
		})
}
