package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func registerPlan(r *Registry) {
	// The recovery source the plan state key declares. A session with no list yet
	// answers with the empty state at revision 0 — "nothing written" is a fact, and
	// only a session that does not exist is an error.
	Query(r, MethodMeta{
		Name:            "plan.get",
		Errors:          []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeaturePlan),
		Stability:       stable,
	}, func(d *Router, ctx context.Context, in protocol.GetPlanRequest) (*protocol.StateSnapshot, error) {
		return d.api.GetPlan(ctx, in)
	})
}
