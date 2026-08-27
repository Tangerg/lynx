package embedded

import (
	"context"

	"github.com/Tangerg/scope/app/runtime/internal/delivery/operation"
	"github.com/Tangerg/scope/app/runtime/protocol"
)

// ListItems returns the authoritative transcript Items for a Session or Run scope.
func (r *Runtime) ListItems(ctx context.Context, request protocol.ListItemsRequest, options CallOptions) (*protocol.ListItemsResponse, error) {
	return r.invoke[protocol.ListItemsRequest, *protocol.ListItemsResponse](ctx, operation.ItemsList, request, callOptions(options))
}
