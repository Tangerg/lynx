package embedded

import (
	"context"

	"github.com/Tangerg/scope/app/runtime/internal/delivery/operation"
	"github.com/Tangerg/scope/app/runtime/protocol"
)

// Discover returns the protocol range and capabilities of this Runtime.
func (r *Runtime) Discover(ctx context.Context, options CallOptions) (*protocol.DiscoverResponse, error) {
	return r.invoke[struct{}, *protocol.DiscoverResponse](ctx, operation.RuntimeDiscover, struct{}{}, callOptions(options))
}
