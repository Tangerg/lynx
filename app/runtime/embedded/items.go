package embedded

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// ListItems returns the authoritative transcript Items for a Session or Run scope.
func (r *Runtime) ListItems(ctx context.Context, request protocol.ListItemsRequest, options CallOptions) (*protocol.ListItemsResponse, error) {
	return invoke[protocol.ListItemsRequest, *protocol.ListItemsResponse](ctx, r, "items.list", request, callOptions(options))
}
