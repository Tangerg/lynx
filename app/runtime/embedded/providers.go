package embedded

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// ListProviders returns the provider registry projection.
func (r *Runtime) ListProviders(ctx context.Context, options CallOptions) (*protocol.Page[protocol.Provider], error) {
	return invoke[struct{}, *protocol.Page[protocol.Provider]](ctx, r, "providers.list", struct{}{}, callOptions(options))
}

// UpdateProvider updates credentials or endpoint settings for one provider.
func (r *Runtime) UpdateProvider(ctx context.Context, request protocol.UpdateProviderRequest, options CommandOptions) (*protocol.Provider, error) {
	return invoke[protocol.UpdateProviderRequest, *protocol.Provider](ctx, r, "providers.update", request, commandOptions(options))
}

// TestProvider probes one provider without mutating its configuration.
func (r *Runtime) TestProvider(ctx context.Context, request protocol.TestProviderRequest, options CallOptions) (*protocol.ProviderTestResult, error) {
	return invoke[protocol.TestProviderRequest, *protocol.ProviderTestResult](ctx, r, "providers.test", request, callOptions(options))
}
