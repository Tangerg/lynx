package embedded

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/operation"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// ListInterrupts returns waiting interrupt sets for Run trees.
func (r *Runtime) ListInterrupts(ctx context.Context, request protocol.ListInterruptsRequest, options CallOptions) (*protocol.Page[protocol.PendingInterruptSet], error) {
	return r.invoke[protocol.ListInterruptsRequest, *protocol.Page[protocol.PendingInterruptSet]](ctx, operation.InterruptsList, request, callOptions(options))
}
