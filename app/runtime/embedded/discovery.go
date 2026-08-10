package embedded

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// Discover returns the protocol range and capabilities of this Runtime.
func (r *Runtime) Discover(ctx context.Context, options CallOptions) (*protocol.DiscoverResponse, error) {
	return invoke[struct{}, *protocol.DiscoverResponse](ctx, r, "runtime.discover", struct{}{}, callOptions(options))
}
