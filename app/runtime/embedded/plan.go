package embedded

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// GetPlan returns the current Plan for a Session.
func (r *Runtime) GetPlan(ctx context.Context, request protocol.GetPlanRequest, options CallOptions) (*protocol.Plan, error) {
	return invoke[protocol.GetPlanRequest, *protocol.Plan](ctx, r, "plan.get", request, callOptions(options))
}
