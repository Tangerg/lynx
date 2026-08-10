package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func registerPlan(registry *Registry) {
	// The recovery source the plan state key declares. A session with no list yet
	// answers with the empty state at revision 0 — "nothing written" is a fact, and
	// only a session that does not exist is an error.
	Query(registry, MethodMeta{
		Name:            "plan.get",
		Errors:          []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeaturePlan),
		Stability:       stable,
	}, func(router *Router, ctx context.Context, request protocol.GetPlanRequest) (*protocol.StateSnapshot, error) {
		return router.api.GetPlan(ctx, request)
	})
}
