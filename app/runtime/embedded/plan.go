package embedded

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/operation"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// GetPlan returns the current Plan for a Session.
func (r *Runtime) GetPlan(ctx context.Context, request protocol.GetPlanRequest, options CallOptions) (*protocol.Plan, error) {
	return r.invoke[protocol.GetPlanRequest, *protocol.Plan](ctx, operation.PlanGet, request, callOptions(options))
}
