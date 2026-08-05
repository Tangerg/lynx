package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func registerGoals(r *Registry) {
	Command(r, MethodMeta{
		Name: "goals.start", Errors: []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureGoals), Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.StartGoalRequest) (*protocol.Goal, error) {
		return d.api.StartGoal(ctx, in)
	})

	Query(r, MethodMeta{
		Name: "goals.get", Errors: []string{protocol.ErrSessionNotFound.Error()},
		// A session with no goal is not an error, so the published result admits null.
		ResultNullable: true, CapabilityRules: requires(protocol.FeatureGoals), Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.GoalRequest) (*protocol.Goal, error) {
		return d.api.GetGoal(ctx, in)
	})

	Command(r, MethodMeta{
		Name: "goals.stop", Errors: []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureGoals), Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.GoalRequest) (*protocol.Goal, error) {
		return d.api.StopGoal(ctx, in)
	})

	Command(r, MethodMeta{
		Name: "goals.resume", Errors: []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureGoals), Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.GoalRequest) (*protocol.Goal, error) {
		return d.api.ResumeGoal(ctx, in)
	})
}
