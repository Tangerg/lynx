package operation

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

const (
	GoalsStart  Name = "goals.start"
	GoalsUpdate Name = "goals.update"
	GoalsClear  Name = "goals.clear"
	GoalsGet    Name = "goals.get"
	GoalsStop   Name = "goals.stop"
	GoalsResume Name = "goals.resume"
)

func registerGoals(registry *Registry) {
	registry.Command(MethodMeta{
		Name: GoalsStart, Errors: []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureGoals),
	}, func(service interface {
		StartGoal(context.Context, protocol.StartGoalRequest) (*protocol.Goal, error)
	}, ctx context.Context, request protocol.StartGoalRequest) (*protocol.Goal, error) {
		return service.StartGoal(ctx, request)
	})

	registry.Command(MethodMeta{
		Name: GoalsUpdate, Errors: []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureGoals),
	}, func(service interface {
		UpdateGoal(context.Context, protocol.UpdateGoalRequest) (*protocol.Goal, error)
	}, ctx context.Context, request protocol.UpdateGoalRequest) (*protocol.Goal, error) {
		return service.UpdateGoal(ctx, request)
	})

	registry.CommandAck(MethodMeta{
		Name: GoalsClear, Errors: []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureGoals),
	}, func(service interface {
		ClearGoal(context.Context, protocol.GoalRequest) error
	}, ctx context.Context, request protocol.GoalRequest) error {
		return service.ClearGoal(ctx, request)
	})

	registry.Query(MethodMeta{
		Name: GoalsGet, Errors: []string{protocol.ErrSessionNotFound.Error()},
		// A session with no goal is not an error, so the published result admits null.
		ResultNullable: true, CapabilityRules: requires(protocol.FeatureGoals),
	}, func(service interface {
		GetGoal(context.Context, protocol.GoalRequest) (*protocol.Goal, error)
	}, ctx context.Context, request protocol.GoalRequest) (*protocol.Goal, error) {
		return service.GetGoal(ctx, request)
	})

	registry.Command(MethodMeta{
		Name: GoalsStop, Errors: []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureGoals),
	}, func(service interface {
		StopGoal(context.Context, protocol.GoalRequest) (*protocol.Goal, error)
	}, ctx context.Context, request protocol.GoalRequest) (*protocol.Goal, error) {
		return service.StopGoal(ctx, request)
	})

	registry.Command(MethodMeta{
		Name: GoalsResume, Errors: []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureGoals),
	}, func(service interface {
		ResumeGoal(context.Context, protocol.GoalRequest) (*protocol.Goal, error)
	}, ctx context.Context, request protocol.GoalRequest) (*protocol.Goal, error) {
		return service.ResumeGoal(ctx, request)
	})
}
