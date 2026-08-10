package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func registerGoals(registry *Registry) {
	Command(registry, MethodMeta{
		Name: "goals.start", Errors: []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureGoals), Stability: stable,
	}, func(router *Router, ctx context.Context, request protocol.StartGoalRequest) (*protocol.Goal, error) {
		return router.api.StartGoal(ctx, request)
	})

	Query(registry, MethodMeta{
		Name: "goals.get", Errors: []string{protocol.ErrSessionNotFound.Error()},
		// A session with no goal is not an error, so the published result admits null.
		ResultNullable: true, CapabilityRules: requires(protocol.FeatureGoals), Stability: stable,
	}, func(router *Router, ctx context.Context, request protocol.GoalRequest) (*protocol.Goal, error) {
		return router.api.GetGoal(ctx, request)
	})

	Command(registry, MethodMeta{
		Name: "goals.stop", Errors: []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureGoals), Stability: stable,
	}, func(router *Router, ctx context.Context, request protocol.GoalRequest) (*protocol.Goal, error) {
		return router.api.StopGoal(ctx, request)
	})

	Command(registry, MethodMeta{
		Name: "goals.resume", Errors: []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureGoals), Stability: stable,
	}, func(router *Router, ctx context.Context, request protocol.GoalRequest) (*protocol.Goal, error) {
		return router.api.ResumeGoal(ctx, request)
	})
}
