package operation

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func registerPlan(registry *Registry) {
	// The Plan's cold read. A session with no list yet answers with the empty Plan at
	// revision 0 — "nothing written" is a fact, and
	// only a session that does not exist is an error.
	Query(registry, MethodMeta{
		Name:            "plan.get",
		Errors:          []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeaturePlan),
	}, func(service interface {
		GetPlan(context.Context, protocol.GetPlanRequest) (*protocol.Plan, error)
	}, ctx context.Context, request protocol.GetPlanRequest) (*protocol.Plan, error) {
		return service.GetPlan(ctx, request)
	})
}
