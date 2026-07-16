package event

import (
	"time"

	"github.com/Tangerg/lynx/agent/core"
)

// ActionStarted fires before an action is invoked (per
// retry attempt the runtime publishes only on the outer call, not per
// retry).
type ActionStarted struct {
	Header
	Action    core.Action `json:"-"`
	StartedAt time.Time   `json:"-"`
}

func (ActionStarted) Kind() string { return "action_started" }

// ActionFinished fires after an action's retry loop
// terminates — Status carries the final outcome, Err the last error
// (may be nil on success).
type ActionFinished struct {
	Header
	Action   core.Action       `json:"-"`
	Status   core.ActionStatus `json:"-"`
	Duration time.Duration     `json:"-"`
	Err      error             `json:"-"`
}

func (ActionFinished) Kind() string { return "action_finished" }

// GoalAchieved fires when the planner returns an empty plan for a
// non-nil goal (i.e. preconditions are already satisfied).
type GoalAchieved struct {
	Header
	Goal *core.Goal `json:"-"`
}

func (GoalAchieved) Kind() string { return "goal_achieved" }
