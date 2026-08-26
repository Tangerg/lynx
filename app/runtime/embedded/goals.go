package embedded

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/operation"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// StartGoal starts autonomous Goal pursuit for a Session.
func (r *Runtime) StartGoal(ctx context.Context, request protocol.StartGoalRequest, options CommandOptions) (*protocol.Goal, error) {
	return r.invoke[protocol.StartGoalRequest, *protocol.Goal](ctx, operation.GoalsStart, request, commandOptions(options))
}

// UpdateGoal revises the current Goal objective.
func (r *Runtime) UpdateGoal(ctx context.Context, request protocol.UpdateGoalRequest, options CommandOptions) (*protocol.Goal, error) {
	return r.invoke[protocol.UpdateGoalRequest, *protocol.Goal](ctx, operation.GoalsUpdate, request, commandOptions(options))
}

// ClearGoal clears autonomous Goal pursuit.
func (r *Runtime) ClearGoal(ctx context.Context, request protocol.GoalRequest, options CommandOptions) error {
	return r.invokeAck(ctx, operation.GoalsClear, request, commandOptions(options))
}

// GetGoal returns the Session's current Goal, or nil when none exists.
func (r *Runtime) GetGoal(ctx context.Context, request protocol.GoalRequest, options CallOptions) (*protocol.Goal, error) {
	return r.invoke[protocol.GoalRequest, *protocol.Goal](ctx, operation.GoalsGet, request, callOptions(options))
}

// StopGoal stops autonomous Goal pursuit.
func (r *Runtime) StopGoal(ctx context.Context, request protocol.GoalRequest, options CommandOptions) (*protocol.Goal, error) {
	return r.invoke[protocol.GoalRequest, *protocol.Goal](ctx, operation.GoalsStop, request, commandOptions(options))
}

// ResumeGoal resumes paused Goal pursuit.
func (r *Runtime) ResumeGoal(ctx context.Context, request protocol.GoalRequest, options CommandOptions) (*protocol.Goal, error) {
	return r.invoke[protocol.GoalRequest, *protocol.Goal](ctx, operation.GoalsResume, request, commandOptions(options))
}
