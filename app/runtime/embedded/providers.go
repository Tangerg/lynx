package embedded

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/operation"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// ListProviders returns the provider registry projection.
func (r *Runtime) ListProviders(ctx context.Context, options CallOptions) (*protocol.Page[protocol.Provider], error) {
	return r.invoke[struct{}, *protocol.Page[protocol.Provider]](ctx, operation.ProvidersList, struct{}{}, callOptions(options))
}

// UpdateProvider updates credentials or endpoint settings for one provider.
func (r *Runtime) UpdateProvider(ctx context.Context, request protocol.UpdateProviderRequest, options CommandOptions) (*protocol.Provider, error) {
	return r.invoke[protocol.UpdateProviderRequest, *protocol.Provider](ctx, operation.ProvidersUpdate, request, commandOptions(options))
}

// TestProvider probes one provider without mutating its configuration.
func (r *Runtime) TestProvider(ctx context.Context, request protocol.TestProviderRequest, options CallOptions) (*protocol.ProviderTestResult, error) {
	return r.invoke[protocol.TestProviderRequest, *protocol.ProviderTestResult](ctx, operation.ProvidersTest, request, callOptions(options))
}
