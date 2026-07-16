package agent

import (
	"context"

	"github.com/Tangerg/lynx/agent/core"
)

// New constructs a read-only Agent definition from ordinary Go config.
func New(config AgentConfig) *Agent { return core.NewAgent(config) }

// NewGoal constructs an immutable Goal from ordinary Go config.
func NewGoal(config GoalConfig) *Goal { return core.NewGoal(config) }

// NewAction constructs a typed function-backed action. Pass [ActionConfig]{}
// when defaults suffice.
func NewAction[In, Out any](name string, fn func(context.Context, *ProcessContext, In) (Out, error), config ActionConfig) *FuncAction[In, Out] {
	return core.NewAction[In, Out](name, core.ActionFunc[In, Out](fn), config)
}

// NewCondition constructs a function-backed condition.
func NewCondition(name string, fn func(context.Context, *ConditionEnv) Truth) *FuncCondition {
	return core.NewCondition(name, fn)
}

// NewOutputGoal constructs a goal whose precondition is an artifact of type T
// on the blackboard.
func NewOutputGoal[T any](config GoalConfig) *Goal { return core.NewOutputGoal[T](config) }
