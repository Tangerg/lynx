package embedded

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// GetPlan returns the current Plan state snapshot for a Session.
func (r *Runtime) GetPlan(ctx context.Context, request protocol.GetPlanRequest, options CallOptions) (*protocol.StateSnapshot, error) {
	return invoke[protocol.GetPlanRequest, *protocol.StateSnapshot](ctx, r, "plan.get", request, callOptions(options))
}
