package event

import (
	"time"

	"github.com/Tangerg/lynx/agent/core"
)

// ActionExecutionStart fires before an action is invoked (per
// retry attempt the runtime publishes only on the outer call, not per
// retry).
type ActionExecutionStart struct {
	BaseEvent
	Action    core.Action `json:"-"`
	StartedAt time.Time   `json:"-"`
}

func (ActionExecutionStart) EventName() string { return "action_execution_start" }

// ActionExecutionResult fires after an action's retry loop
// terminates — Status carries the final outcome, Err the last error
// (may be nil on success).
type ActionExecutionResult struct {
	BaseEvent
	Action   core.Action       `json:"-"`
	Status   core.ActionStatus `json:"-"`
	Duration time.Duration     `json:"-"`
	Err      error             `json:"-"`
}

func (ActionExecutionResult) EventName() string { return "action_execution_result" }

// GoalAchieved fires when the planner returns an empty plan for a
// non-nil goal (i.e. preconditions are already satisfied).
type GoalAchieved struct {
	BaseEvent
	Goal *core.Goal `json:"-"`
}

func (GoalAchieved) EventName() string { return "goal_achieved" }
