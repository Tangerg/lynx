package embedded

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// ListInterrupts returns waiting interrupt sets for Run trees.
func (r *Runtime) ListInterrupts(ctx context.Context, request protocol.ListInterruptsRequest, options CallOptions) (*protocol.Page[protocol.PendingInterruptSet], error) {
	return invoke[protocol.ListInterruptsRequest, *protocol.Page[protocol.PendingInterruptSet]](ctx, r, "interrupts.list", request, callOptions(options))
}
