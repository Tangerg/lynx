package embedded

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// StartGoal starts autonomous Goal pursuit for a Session.
func (r *Runtime) StartGoal(ctx context.Context, request protocol.StartGoalRequest, options CommandOptions) (*protocol.Goal, error) {
	return invoke[protocol.StartGoalRequest, *protocol.Goal](ctx, r, "goals.start", request, commandOptions(options))
}

// GetGoal returns the Session's current Goal, or nil when none exists.
func (r *Runtime) GetGoal(ctx context.Context, request protocol.GoalRequest, options CallOptions) (*protocol.Goal, error) {
	return invoke[protocol.GoalRequest, *protocol.Goal](ctx, r, "goals.get", request, callOptions(options))
}

// StopGoal stops autonomous Goal pursuit.
func (r *Runtime) StopGoal(ctx context.Context, request protocol.GoalRequest, options CommandOptions) (*protocol.Goal, error) {
	return invoke[protocol.GoalRequest, *protocol.Goal](ctx, r, "goals.stop", request, commandOptions(options))
}

// ResumeGoal resumes paused Goal pursuit.
func (r *Runtime) ResumeGoal(ctx context.Context, request protocol.GoalRequest, options CommandOptions) (*protocol.Goal, error) {
	return invoke[protocol.GoalRequest, *protocol.Goal](ctx, r, "goals.resume", request, commandOptions(options))
}
