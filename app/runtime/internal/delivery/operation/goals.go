package operation

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func registerGoals(registry *Registry) {
	Command(registry, MethodMeta{
		Name: "goals.start", Errors: []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureGoals),
	}, func(service interface {
		StartGoal(context.Context, protocol.StartGoalRequest) (*protocol.Goal, error)
	}, ctx context.Context, request protocol.StartGoalRequest) (*protocol.Goal, error) {
		return service.StartGoal(ctx, request)
	})

	Command(registry, MethodMeta{
		Name: "goals.update", Errors: []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureGoals),
	}, func(service interface {
		UpdateGoal(context.Context, protocol.UpdateGoalRequest) (*protocol.Goal, error)
	}, ctx context.Context, request protocol.UpdateGoalRequest) (*protocol.Goal, error) {
		return service.UpdateGoal(ctx, request)
	})

	CommandAck(registry, MethodMeta{
		Name: "goals.clear", Errors: []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureGoals),
	}, func(service interface {
		ClearGoal(context.Context, protocol.GoalRequest) error
	}, ctx context.Context, request protocol.GoalRequest) error {
		return service.ClearGoal(ctx, request)
	})

	Query(registry, MethodMeta{
		Name: "goals.get", Errors: []string{protocol.ErrSessionNotFound.Error()},
		// A session with no goal is not an error, so the published result admits null.
		ResultNullable: true, CapabilityRules: requires(protocol.FeatureGoals),
	}, func(service interface {
		GetGoal(context.Context, protocol.GoalRequest) (*protocol.Goal, error)
	}, ctx context.Context, request protocol.GoalRequest) (*protocol.Goal, error) {
		return service.GetGoal(ctx, request)
	})

	Command(registry, MethodMeta{
		Name: "goals.stop", Errors: []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureGoals),
	}, func(service interface {
		StopGoal(context.Context, protocol.GoalRequest) (*protocol.Goal, error)
	}, ctx context.Context, request protocol.GoalRequest) (*protocol.Goal, error) {
		return service.StopGoal(ctx, request)
	})

	Command(registry, MethodMeta{
		Name: "goals.resume", Errors: []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureGoals),
	}, func(service interface {
		ResumeGoal(context.Context, protocol.GoalRequest) (*protocol.Goal, error)
	}, ctx context.Context, request protocol.GoalRequest) (*protocol.Goal, error) {
		return service.ResumeGoal(ctx, request)
	})
}
